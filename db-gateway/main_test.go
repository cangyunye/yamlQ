package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonFlagImpliesServiceMode(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		daemon bool
		want   string
	}{
		{"daemon overrides cli", "cli", true, "service"},
		{"daemon keeps service", "service", true, "service"},
		{"no daemon keeps cli", "cli", false, "cli"},
		{"no daemon keeps service", "service", false, "service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMode(tt.mode, tt.daemon)
			if got != tt.want {
				t.Errorf("resolveMode(%q, %v) = %q, want %q", tt.mode, tt.daemon, got, tt.want)
			}
		})
	}
}

func TestWriteEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	writeEnvFile(8080, "secret-token", true)

	path := filepath.Join(tmpDir, ".yamlq-gateway.env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read env file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "YAMLQ_GATEWAY_URL=http://127.0.0.1:8080") {
		t.Errorf("missing URL in env file:\n%s", content)
	}
	if !strings.Contains(content, "YAMLQ_GATEWAY_AUTH=secret-token") {
		t.Errorf("missing auth token in env file:\n%s", content)
	}
	if !strings.Contains(content, "YAMLQ_GATEWAY_DAEMON=1") {
		t.Errorf("missing daemon flag in env file:\n%s", content)
	}
}

func TestWriteEnvFileNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	writeEnvFile(3000, "", false)

	path := filepath.Join(tmpDir, ".yamlq-gateway.env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read env file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "YAMLQ_GATEWAY_URL=http://127.0.0.1:3000") {
		t.Errorf("missing URL in env file:\n%s", content)
	}
	if strings.Contains(content, "YAMLQ_GATEWAY_AUTH") {
		t.Errorf("auth should not appear when token is empty:\n%s", content)
	}
	if strings.Contains(content, "YAMLQ_GATEWAY_DAEMON") {
		t.Errorf("daemon flag should not appear when daemon=false:\n%s", content)
	}
}

func TestEnvFileDeletedOnShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	writeEnvFile(8080, "", true)
	path := filepath.Join(tmpDir, ".yamlq-gateway.env")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("env file should exist after writeEnvFile")
	}

	cleanupEnvFile()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("env file should be removed after cleanupEnvFile")
	}
}
