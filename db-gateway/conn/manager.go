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
	done       chan struct{}
	cancel     context.CancelFunc
}

type Manager struct {
	mu        sync.Mutex
	conns     map[string]*ManagedConn
	cfg       config.PoolConfig
	globalSem chan struct{}
	querySem  chan struct{}
	nextID    int64
}

func NewManager(cfg config.PoolConfig) *Manager {
	return &Manager{
		conns:     make(map[string]*ManagedConn),
		cfg:       cfg,
		globalSem: make(chan struct{}, cfg.GlobalMaxOpen),
		querySem:  make(chan struct{}, cfg.MaxConcurrentQuery),
	}
}

func (m *Manager) OpenConn(driverName, dsn string, maxOpen, maxIdle, lifetimeSec int) (string, error) {
	select {
	case m.globalSem <- struct{}{}:
	default:
		return "", fmt.Errorf("global connection limit reached")
	}

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
		done:       make(chan struct{}),
		cancel:     cancel,
	}
	m.conns[connID] = mc
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

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conns {
		c.cancel()
		close(c.done)
		c.DB.Close()
		delete(m.conns, id)
		<-m.globalSem
	}
}

func (m *Manager) CloseConn(connID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, ok := m.conns[connID]
	if !ok {
		return fmt.Errorf("connection not found: %s", connID)
	}
	mc.cancel()
	close(mc.done)
	mc.DB.Close()
	delete(m.conns, connID)
	<-m.globalSem
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
