package conn

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"yamlq/db-gateway/config"
	"yamlq/db-gateway/driver"
	"yamlq/db-gateway/errs"
)

type QueryTask struct {
	SQL        string
	Params     []interface{}
	Page       int
	PageSize   int
	Timeout    time.Duration
	ResultChan chan *QueryResult
}

type ManagedConn struct {
	DB         *sql.DB
	Driver     string
	DSN        string
	CreatedAt  time.Time
	LastActive time.Time
	QueryChan  chan *QueryTask
	key        string
	done       chan struct{}
	cancel     context.CancelFunc
}

type Manager struct {
	mu         sync.Mutex
	conns      map[string]*ManagedConn
	keys       map[string]string
	cfg        config.PoolConfig
	globalSem  chan struct{}
	querySem   chan struct{}
	nextID     int64
	reaperStop chan struct{}
	reaperDone chan struct{}
}

func NewManager(cfg config.PoolConfig) *Manager {
	return &Manager{
		conns:     make(map[string]*ManagedConn),
		keys:      make(map[string]string),
		cfg:       cfg,
		globalSem: make(chan struct{}, cfg.GlobalMaxOpen),
		querySem:  make(chan struct{}, cfg.MaxConcurrentQuery),
	}
}

// connKey identifies a pool by driver+DSN so repeated /connect calls for the
// same database (e.g. every CLI run against a resident gateway) reuse the
// existing pool instead of opening a duplicate one.
func connKey(driverName, dsn string) string {
	return driverName + "\x00" + dsn
}

func (m *Manager) OpenConn(driverName, dsn string, maxOpen, maxIdle, lifetimeSec int) (string, error) {
	key := connKey(driverName, dsn)

	m.mu.Lock()
	if connID, ok := m.keys[key]; ok {
		m.mu.Unlock()
		return connID, nil
	}
	m.mu.Unlock()

	// The only global slot may be held by a concurrent opener for the same
	// key: wait briefly for it to register the pool, retrying the semaphore
	// in case the slot frees up, and give up only on real exhaustion.
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case m.globalSem <- struct{}{}:
			return m.dial(key, driverName, dsn, maxOpen, maxIdle, lifetimeSec)
		default:
		}
		m.mu.Lock()
		connID, ok := m.keys[key]
		m.mu.Unlock()
		if ok {
			return connID, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("global connection limit reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (m *Manager) dial(key, driverName, dsn string, maxOpen, maxIdle, lifetimeSec int) (string, error) {
	db, err := driver.Open(driverName, dsn)
	if err != nil {
		<-m.globalSem
		return "", fmt.Errorf("failed to open driver %s: %w", driverName, driver.AnnotateConnectError(driverName, dsn, err))
	}

	if maxOpen <= 0 {
		maxOpen = m.cfg.PerConnMaxOpen
	}
	if maxIdle <= 0 {
		maxIdle = m.cfg.PerConnMaxIdle
	}
	if lifetimeSec <= 0 {
		lifetimeSec = m.cfg.ConnMaxLifetimeSec
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(lifetimeSec) * time.Second)

	if err := db.Ping(); err != nil {
		db.Close()
		<-m.globalSem
		return "", fmt.Errorf("connection ping failed: %w", driver.AnnotateConnectError(driverName, dsn, err))
	}

	ctx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	// A concurrent OpenConn for the same key may have won the race while we
	// were dialing; keep exactly one pool per driver+DSN.
	if connID, ok := m.keys[key]; ok {
		m.mu.Unlock()
		cancel()
		db.Close()
		<-m.globalSem
		return connID, nil
	}
	m.nextID++
	connID := fmt.Sprintf("conn_%d_%d", time.Now().Unix(), m.nextID)
	now := time.Now()
	mc := &ManagedConn{
		DB:         db,
		Driver:     driverName,
		DSN:        dsn,
		CreatedAt:  now,
		LastActive: now,
		QueryChan:  make(chan *QueryTask, m.cfg.QueueDepth),
		key:        key,
		done:       make(chan struct{}),
		cancel:     cancel,
	}
	m.conns[connID] = mc
	m.keys[key] = connID
	m.mu.Unlock()

	go m.consumeQueries(ctx, connID, mc)

	return connID, nil
}

func (m *Manager) consumeQueries(ctx context.Context, connID string, mc *ManagedConn) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-mc.QueryChan:
			if !ok {
				return
			}
			m.querySem <- struct{}{}
			result := m.executeOne(mc, task)
			<-m.querySem
			task.ResultChan <- result
		}
	}
}

func (m *Manager) SubmitQuery(connID, sqlStr string, params []interface{}, page, pageSize int, timeout time.Duration) (*QueryResult, error) {
	m.mu.Lock()
	mc, ok := m.conns[connID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("connection not found: %s", connID)
	}

	task := &QueryTask{
		SQL:        sqlStr,
		Params:     params,
		Page:       page,
		PageSize:   pageSize,
		Timeout:    timeout,
		ResultChan: make(chan *QueryResult, 1),
	}

	select {
	case mc.QueryChan <- task:
	default:
		return nil, &errs.QueueFullError{Depth: m.cfg.QueueDepth}
	}

	select {
	case result := <-task.ResultChan:
		return result, nil
	case <-mc.done:
		return nil, fmt.Errorf("connection closed while waiting for result")
	}
}

// closeLocked tears down a pool. Callers must hold m.mu.
func (m *Manager) closeLocked(connID string, mc *ManagedConn) {
	delete(m.conns, connID)
	if mc.key != "" && m.keys[mc.key] == connID {
		delete(m.keys, mc.key)
	}
	mc.cancel()
	close(mc.done)
	mc.DB.Close()
	<-m.globalSem
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conns {
		m.closeLocked(id, c)
	}
}

func (m *Manager) CloseConn(connID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, ok := m.conns[connID]
	if !ok {
		return fmt.Errorf("connection not found: %s", connID)
	}
	m.closeLocked(connID, mc)
	return nil
}

func (m *Manager) Ping(connID string) error {
	m.mu.Lock()
	mc, ok := m.conns[connID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("connection not found: %s", connID)
	}
	return mc.DB.Ping()
}

// StartReaper launches a background goroutine for long-lived (serve) mode:
// pools idle longer than idleTimeout are closed, and pools idle past half the
// timeout get a liveness probe so dead pools are detected before the next
// client query hits them. The reaper never touches pools with recent activity.
func (m *Manager) StartReaper(idleTimeout, interval time.Duration) {
	m.mu.Lock()
	if m.reaperStop != nil || idleTimeout <= 0 || interval <= 0 {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	m.reaperStop = stop
	m.reaperDone = done
	m.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.reapOnce(idleTimeout)
			}
		}
	}()
}

func (m *Manager) StopReaper() {
	m.mu.Lock()
	stop, done := m.reaperStop, m.reaperDone
	m.reaperStop, m.reaperDone = nil, nil
	m.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func (m *Manager) reapOnce(idleTimeout time.Duration) {
	now := time.Now()

	m.mu.Lock()
	var stale []string
	var toProbe []*ManagedConn
	for _, mc := range m.conns {
		idle := now.Sub(mc.LastActive)
		switch {
		case idle >= idleTimeout:
			stale = append(stale, mc.key)
		case idle >= idleTimeout/2:
			toProbe = append(toProbe, mc)
		}
	}
	for _, key := range stale {
		if connID, ok := m.keys[key]; ok {
			if mc, ok := m.conns[connID]; ok {
				m.closeLocked(connID, mc)
			}
		}
	}
	m.mu.Unlock()

	for _, mc := range toProbe {
		if err := m.probe(mc, 5*time.Second); err != nil {
			m.mu.Lock()
			if current, ok := m.conns[mc.key]; ok && current == mc {
				m.closeLocked(m.keys[mc.key], mc)
			}
			m.mu.Unlock()
		}
	}
}

// probe pings a pool with a bounded timeout. It does not refresh LastActive:
// reaping is driven by client activity, not by the reaper's own pings.
func (m *Manager) probe(mc *ManagedConn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return mc.DB.PingContext(ctx)
}
