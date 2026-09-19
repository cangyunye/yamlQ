package conn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yamlq/db-gateway/config"
	gwdriver "yamlq/db-gateway/driver"
)

// A minimal in-process driver so manager tests never need a real database.
// Per-DSN behavior (ping success/failure) is controlled through fakeStates.

type fakeState struct {
	pingOK  atomic.Bool
	closed  atomic.Int64
	dialed  atomic.Int64
	pinged  atomic.Int64
	queried atomic.Int64
}

var fakeStates sync.Map // dsn -> *fakeState

func fakeStateFor(dsn string) *fakeState {
	if v, ok := fakeStates.Load(dsn); ok {
		return v.(*fakeState)
	}
	st := &fakeState{}
	st.pingOK.Store(true)
	actual, _ := fakeStates.LoadOrStore(dsn, st)
	return actual.(*fakeState)
}

type fakeDriver struct{}

func (fakeDriver) Open(dsn string) (driver.Conn, error) {
	st := fakeStateFor(dsn)
	st.dialed.Add(1)
	return &fakeConn{state: st}, nil
}

type fakeConn struct {
	state *fakeState
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("fake driver: Prepare not supported")
}
func (c *fakeConn) Close() error {
	c.state.closed.Add(1)
	return nil
}
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("fake driver: Begin not supported") }

func (c *fakeConn) Ping(ctx context.Context) error {
	c.state.pinged.Add(1)
	if !c.state.pingOK.Load() {
		return errors.New("fake ping failed")
	}
	return nil
}

func (c *fakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.queried.Add(1)
	if !c.state.pingOK.Load() {
		return nil, errors.New("fake query failed")
	}
	return &fakeRows{}, nil
}

type fakeRows struct{}

func (fakeRows) Columns() []string { return []string{"x"} }
func (fakeRows) Close() error      { return nil }
func (fakeRows) Next(dest []driver.Value) error {
	return io.EOF
}

func init() {
	sql.Register("fake-sql", fakeDriver{})
	gwdriver.Register("yamlq-fake", func(dsn string) (*sql.DB, error) {
		return sql.Open("fake-sql", dsn)
	})
}

func fakeManager() *Manager {
	cfg := config.PoolConfig{
		GlobalMaxOpen:      1,
		PerConnMaxOpen:     2,
		PerConnMaxIdle:     1,
		MaxConcurrentQuery: 4,
		QueueDepth:         4,
		ConnMaxLifetimeSec: 60,
		MaxRowsPerQuery:    100,
	}
	return NewManager(cfg)
}

func TestOpenConnIdempotent(t *testing.T) {
	mgr := fakeManager()
	defer mgr.CloseAll()

	id1, err := mgr.OpenConn("yamlq-fake", "fake:idem", 0, 0, 0)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	id2, err := mgr.OpenConn("yamlq-fake", "fake:idem", 0, 0, 0)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("same driver+dsn must reuse the pool: got %s and %s", id1, id2)
	}
	if got := fakeStateFor("fake:idem").dialed.Load(); got != 1 {
		t.Fatalf("expected exactly one dial, got %d", got)
	}

	// The pool holds the single globalSem slot, so a different DSN cannot open.
	if _, err := mgr.OpenConn("yamlq-fake", "fake:other", 0, 0, 0); err == nil || !strings.Contains(err.Error(), "global connection limit") {
		t.Fatalf("expected global connection limit, got %v", err)
	}

	if err := mgr.CloseConn(id1); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := mgr.CloseConn(id1); err == nil {
		t.Fatal("closing an already-closed conn must fail")
	}

	// The slot and the key entry are freed: a fresh pool can open again.
	id3, err := mgr.OpenConn("yamlq-fake", "fake:idem", 0, 0, 0)
	if err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	if id3 == id1 {
		t.Fatal("expected a new conn id after close+reopen")
	}
}

func TestOpenConnConcurrentSameDSN(t *testing.T) {
	mgr := fakeManager()
	defer mgr.CloseAll()

	const n = 8
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := mgr.OpenConn("yamlq-fake", "fake:race", 0, 0, 0)
			if err != nil {
				t.Errorf("open: %v", err)
				return
			}
			ids[i] = id
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent opens returned different conn ids: %s vs %s", ids[0], ids[i])
		}
	}
	if got := fakeStateFor("fake:race").dialed.Load(); got != 1 {
		t.Fatalf("expected exactly one dial under concurrency, got %d", got)
	}
}

func TestReaperClosesIdle(t *testing.T) {
	mgr := fakeManager()
	defer mgr.StopReaper()
	defer mgr.CloseAll()

	id, err := mgr.OpenConn("yamlq-fake", "fake:idle", 0, 0, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mgr.StartReaper(150*time.Millisecond, 50*time.Millisecond)

	deadline := time.Now().Add(3 * time.Second)
	for {
		mgr.mu.Lock()
		_, alive := mgr.conns[id]
		mgr.mu.Unlock()
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reaper did not close the idle pool in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Key entry cleaned: reopening builds a fresh pool instead of returning
	// the reaped conn id.
	again, err := mgr.OpenConn("yamlq-fake", "fake:idle", 0, 0, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if again == id {
		t.Fatal("expected a new conn id after the reaper closed the pool")
	}
}

func TestReaperProbeClosesDead(t *testing.T) {
	mgr := fakeManager()
	defer mgr.StopReaper()
	defer mgr.CloseAll()

	st := fakeStateFor("fake:dead")
	id, err := mgr.OpenConn("yamlq-fake", "fake:dead", 0, 0, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mgr.StartReaper(1*time.Second, 40*time.Millisecond)

	// Pool is idle past half the timeout; once pings start failing the reaper
	// must retire the dead pool.
	time.Sleep(600 * time.Millisecond)
	st.pingOK.Store(false)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := mgr.Ping(id); err != nil && strings.Contains(err.Error(), "connection not found") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reaper did not retire the dead pool in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestReaperRespectsActive(t *testing.T) {
	mgr := fakeManager()
	defer mgr.StopReaper()
	defer mgr.CloseAll()

	id, err := mgr.OpenConn("yamlq-fake", "fake:busy", 0, 0, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mgr.StartReaper(300*time.Millisecond, 50*time.Millisecond)

	// Keep the pool active well below the idle timeout.
	for i := 0; i < 6; i++ {
		res, err := mgr.SubmitQuery(id, "SELECT 1", nil, 0, 0, time.Second)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if res.Error != nil {
			t.Fatalf("query error: %v", res.Error)
		}
		time.Sleep(100 * time.Millisecond)
	}

	mgr.mu.Lock()
	_, alive := mgr.conns[id]
	mgr.mu.Unlock()
	if !alive {
		t.Fatal("actively used pool must not be reaped")
	}
}

func TestCloseAllCleansKeys(t *testing.T) {
	mgr := fakeManager()
	defer mgr.StopReaper()

	id, err := mgr.OpenConn("yamlq-fake", "fake:all", 0, 0, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	mgr.CloseAll()

	mgr.mu.Lock()
	_, hasConn := mgr.conns[id]
	_, hasKey := mgr.keys[connKey("yamlq-fake", "fake:all")]
	mgr.mu.Unlock()
	if hasConn || hasKey {
		t.Fatal("CloseAll must clean both conns and keys")
	}

	reopened, err := mgr.OpenConn("yamlq-fake", "fake:all", 0, 0, 0)
	if err != nil {
		t.Fatalf("reopen after CloseAll: %v", err)
	}
	if reopened == id {
		t.Fatal("expected a fresh conn id after CloseAll")
	}
	mgr.CloseAll()
}
