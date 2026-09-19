package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"yamlq/db-gateway/conn"
	"yamlq/db-gateway/config"
)

// testDSN is overridable so the suite can run against any local MySQL:
// YAMLQ_TEST_MYSQL_DSN=root:pw@tcp(host:port)/db go test ./...
var testDSN = func() string {
	if v := os.Getenv("YAMLQ_TEST_MYSQL_DSN"); v != "" {
		return v
	}
	return "root:root123456@tcp(127.0.0.1:3306)/default_db"
}()

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mgr := conn.NewManager(config.CLI)
	srv := New(mgr)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	t.Cleanup(mgr.CloseAll)
	return ts
}

func TestConnectSuccess(t *testing.T) {
	ts := setupTestServer(t)

	body := map[string]interface{}{
		"driver": "mysql",
		"dsn":    testDSN,
	}
	resp := doPost(t, ts.URL+"/connect", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	connID, ok := result["conn_id"].(string)
	if !ok || connID == "" {
		t.Fatalf("expected non-empty conn_id, got %v", result)
	}
	if errMsg, _ := result["error"].(string); errMsg != "" {
		t.Fatalf("expected empty error, got %q", errMsg)
	}
}

func TestQuerySuccess(t *testing.T) {
	ts := setupTestServer(t)

	connResp := doPost(t, ts.URL+"/connect", map[string]interface{}{
		"driver": "mysql",
		"dsn":    testDSN,
	})
	var cr map[string]interface{}
	json.NewDecoder(connResp.Body).Decode(&cr)
	connResp.Body.Close()
	connID := cr["conn_id"].(string)

	queryResp := doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID,
		"sql":     "SELECT 1 AS val, 'hello' AS msg",
		"timeout": 10,
	})
	defer queryResp.Body.Close()

	if queryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", queryResp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(queryResp.Body).Decode(&result)

	if result["error"] != nil {
		t.Fatalf("expected nil error, got %v", result["error"])
	}

	cols, ok := result["columns"].([]interface{})
	if !ok || len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %v", result["columns"])
	}
	if cols[0] != "val" || cols[1] != "msg" {
		t.Fatalf("expected [val msg], got %v", cols)
	}

	rows, ok := result["rows"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("expected 1 row, got %v", result["rows"])
	}
	row := rows[0].([]interface{})
	if row[1] != "hello" {
		t.Fatalf("expected 'hello', got %v", row[1])
	}
}

func TestCloseConnection(t *testing.T) {
	ts := setupTestServer(t)

	connResp := doPost(t, ts.URL+"/connect", map[string]interface{}{
		"driver": "mysql",
		"dsn":    testDSN,
	})
	var cr map[string]interface{}
	json.NewDecoder(connResp.Body).Decode(&cr)
	connResp.Body.Close()
	connID := cr["conn_id"].(string)

	closeResp := doPost(t, ts.URL+"/close", map[string]interface{}{"conn_id": connID})
	defer closeResp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(closeResp.Body).Decode(&result)
	if errMsg, _ := result["error"].(string); errMsg != "" {
		t.Fatalf("expected empty error, got %q", errMsg)
	}

	queryResp := doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID,
		"sql":     "SELECT 1",
		"timeout": 5,
	})
	defer queryResp.Body.Close()
	var qr map[string]interface{}
	json.NewDecoder(queryResp.Body).Decode(&qr)
	if qr["error"] == nil {
		t.Fatal("expected error after close, got nil")
	}
}

func TestPing(t *testing.T) {
	ts := setupTestServer(t)

	connResp := doPost(t, ts.URL+"/connect", map[string]interface{}{
		"driver": "mysql",
		"dsn":    testDSN,
	})
	var cr map[string]interface{}
	json.NewDecoder(connResp.Body).Decode(&cr)
	connResp.Body.Close()
	connID := cr["conn_id"].(string)

	resp, err := http.Get(ts.URL + "/ping?conn_id=" + connID)
	if err != nil {
		t.Fatalf("ping request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result)
	}
}

func TestQueryDBError(t *testing.T) {
	ts := setupTestServer(t)
	connID := connectMySQL(t, ts)

	resp := doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID,
		"sql":     "SELECT * FROM nonexistent_table_xyz",
		"timeout": 10,
	})
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	errObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", result)
	}
	if errObj["code"] != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errObj["code"])
	}
}

func TestQueryTimeout(t *testing.T) {
	ts := setupTestServer(t)
	connID := connectMySQL(t, ts)

	resp := doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID,
		"sql":     "SELECT SLEEP(10)",
		"timeout": 1,
	})
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	errObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", result)
	}
	if errObj["code"] != "QUERY_TIMEOUT" {
		t.Fatalf("expected QUERY_TIMEOUT, got %v", errObj["code"])
	}
}

func TestQueueFull(t *testing.T) {
	mgr := conn.NewManager(config.PoolConfig{
		GlobalMaxOpen:      5,
		PerConnMaxOpen:     2,
		PerConnMaxIdle:     1,
		MaxConcurrentQuery: 5,
		QueueDepth:         1,
		ConnMaxLifetimeSec: 60,
		MaxRowsPerQuery:    100,
	})
	srv := New(mgr)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer mgr.CloseAll()

	connID := connectMySQL(t, ts)

	go doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID, "sql": "SELECT SLEEP(5)", "timeout": 10,
	})
	time.Sleep(100 * time.Millisecond)

	go doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID, "sql": "SELECT SLEEP(5)", "timeout": 10,
	})
	time.Sleep(100 * time.Millisecond)

	resp := doPost(t, ts.URL+"/query", map[string]interface{}{
		"conn_id": connID, "sql": "SELECT 1", "timeout": 5,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errObj := result["error"].(map[string]interface{})
	if errObj["code"] != "QUEUE_FULL" {
		t.Fatalf("expected QUEUE_FULL, got %v", errObj["code"])
	}
}

func connectMySQL(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp := doPost(t, ts.URL+"/connect", map[string]interface{}{
		"driver": "mysql",
		"dsn":    testDSN,
	})
	var cr map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&cr)
	resp.Body.Close()
	return cr["conn_id"].(string)
}

func TestServeModeServesOpenAPI(t *testing.T) {
	srv := New(nil)
	srv.SetServeMode(true)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var spec map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&spec)
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("expected openapi 3.0.3, got %v", spec["openapi"])
	}
}

func TestServeModeServesDocs(t *testing.T) {
	srv := New(nil)
	srv.SetServeMode(true)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "swagger-ui") {
		t.Fatal("expected swagger-ui in response")
	}
}

func TestNonServeModeNoDocs(t *testing.T) {
	srv := New(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/docs")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 without serve mode, got %d", resp.StatusCode)
	}
}

func doPost(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}
