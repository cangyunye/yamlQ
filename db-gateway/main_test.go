package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func awaitReady(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server on port %d did not become ready within %v", port, timeout)
}

func shutdownAndCleanup(t *testing.T, port int) {
	t.Helper()
	http.Get(fmt.Sprintf("http://127.0.0.1:%d/shutdown", port))
	os.Remove(filepath.Join(getwd(t), ".yamlq-gateway.env"))
}

func TestRunServeRespondsToPing(t *testing.T) {
	port := freePort(t)
	done := make(chan struct{}, 1)
	go func() {
		runServe(port, "")
		done <- struct{}{}
	}()
	awaitReady(t, port, 2*time.Second)
	defer shutdownAndCleanup(t, port)
	<-done
}

func TestRunServeWritesEnvFileWithDaemon(t *testing.T) {
	port := freePort(t)
	done := make(chan struct{}, 1)
	go func() {
		runServe(port, "")
		done <- struct{}{}
	}()
	awaitReady(t, port, 2*time.Second)
	defer shutdownAndCleanup(t, port)
	<-done

	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, ".yamlq-gateway.env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("expected env file at %s, got: %v", envPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "YAMLQ_GATEWAY_DAEMON=1") {
		t.Fatalf("expected YAMLQ_GATEWAY_DAEMON=1 in env file, got: %s", content)
	}
	if !strings.Contains(content, fmt.Sprintf("YAMLQ_GATEWAY_URL=http://127.0.0.1:%d", port)) {
		t.Fatalf("expected YAMLQ_GATEWAY_URL in env file, got: %s", content)
	}
}

func TestRunServeWritesAuthTokenInEnvFile(t *testing.T) {
	port := freePort(t)
	authToken := "test-token-123"
	done := make(chan struct{}, 1)
	go func() {
		runServe(port, authToken)
		done <- struct{}{}
	}()
	awaitReady(t, port, 2*time.Second)
	defer shutdownAndCleanup(t, port)
	<-done

	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, ".yamlq-gateway.env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("expected env file, got: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "YAMLQ_GATEWAY_AUTH=test-token-123") {
		t.Fatalf("expected auth token in env file, got: %s", content)
	}
}

func TestRunServePingReturnsOk(t *testing.T) {
	port := freePort(t)
	done := make(chan struct{}, 1)
	go func() {
		runServe(port, "")
		done <- struct{}{}
	}()
	awaitReady(t, port, 2*time.Second)
	defer shutdownAndCleanup(t, port)
	<-done

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func getwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	return wd
}
