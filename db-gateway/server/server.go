package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"yamlq/db-gateway/conn"
	"yamlq/db-gateway/errs"
)

type Server struct {
	mgr          *conn.Manager
	authToken    string
	shutdownFunc func()
}

func New(mgr *conn.Manager) *Server {
	return &Server{mgr: mgr}
}

func (s *Server) SetAuthToken(token string) {
	s.authToken = token
}

func (s *Server) SetShutdownFunc(fn func()) {
	s.shutdownFunc = fn
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /connect", s.handleConnect)
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("POST /close", s.handleClose)
	mux.HandleFunc("GET /ping", s.handlePing)
	mux.HandleFunc("POST /shutdown", s.handleShutdown)
	if s.authToken != "" {
		return s.authMiddleware(mux)
	}
	return mux
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != s.authToken {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type connectRequest struct {
	Driver          string `json:"driver"`
	DSN             string `json:"dsn"`
	MaxOpenConns    int    `json:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
}

type connectResponse struct {
	ConnID string `json:"conn_id"`
	Error  string `json:"error"`
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusOK, connectResponse{Error: "invalid request body"})
		return
	}
	connID, err := s.mgr.OpenConn(req.Driver, req.DSN, req.MaxOpenConns, req.MaxIdleConns, req.ConnMaxLifetime)
	if err != nil {
		writeJSON(w, http.StatusOK, connectResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, connectResponse{ConnID: connID})
}

type queryRequest struct {
	ConnID   string        `json:"conn_id"`
	SQL      string        `json:"sql"`
	Params   []interface{} `json:"params"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Timeout  int           `json:"timeout"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"error": "invalid request body"})
		return
	}
	timeout := time.Duration(req.Timeout) * time.Second
	result, err := s.mgr.SubmitQuery(req.ConnID, req.SQL, req.Params, req.Page, req.PageSize, timeout)
	if err != nil {
		var qf *errs.QueueFullError
		if errors.As(err, &qf) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status": "rejected",
				"error": &errs.Detail{
					Code:    "QUEUE_FULL",
					Message: qf.Error(),
					Hint:    "too many concurrent queries on same connection, retry later",
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"error": &errs.Detail{Code: "CONNECTION_NOT_FOUND", Message: err.Error()},
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConnID string `json:"conn_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := s.mgr.CloseConn(req.ConnID); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"error": ""})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	connID := r.URL.Query().Get("conn_id")
	if err := s.mgr.Ping(connID); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "lost", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"error": ""})
	if s.shutdownFunc != nil {
		go s.shutdownFunc()
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
