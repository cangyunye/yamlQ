package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"yamlq/db-gateway/config"
	"yamlq/db-gateway/conn"
	"yamlq/db-gateway/server"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	mode := flag.String("mode", "cli", "run mode: cli or service")
	authToken := flag.String("auth-token", "", "optional auth token for API access")
	daemonMode := flag.Bool("daemon", false, "run as daemon (stay resident after client disconnects)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("yamlq/db-gateway %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	*mode = resolveMode(*mode, *daemonMode)

	cfg := config.CLI
	if *mode == "service" {
		cfg = config.Service
	}

	mgr := conn.NewManager(cfg)
	srv := server.New(mgr)
	if *authToken != "" {
		srv.SetAuthToken(*authToken)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("listening on port %d\n", port)

	if *daemonMode {
		writeEnvFile(port, *authToken, *daemonMode)
	}

	httpServer := &http.Server{Handler: srv.Handler()}

	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	srv.SetShutdownFunc(func() {
		cleanupEnvFile()
		mgr.CloseAll()
		httpServer.Shutdown(context.Background())
		os.Exit(0)
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cleanupEnvFile()
	mgr.CloseAll()
	httpServer.Shutdown(context.Background())
}

func resolveMode(mode string, daemon bool) string {
	if daemon {
		return "service"
	}
	return mode
}

func writeEnvFile(port int, authToken string, daemon bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path := filepath.Join(cwd, ".yamlq-gateway.env")
	lines := []string{
		fmt.Sprintf("YAMLQ_GATEWAY_URL=http://127.0.0.1:%d", port),
	}
	if authToken != "" {
		lines = append(lines, fmt.Sprintf("YAMLQ_GATEWAY_AUTH=%s", authToken))
	}
	if daemon {
		lines = append(lines, "YAMLQ_GATEWAY_DAEMON=1")
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func cleanupEnvFile() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path := filepath.Join(cwd, ".yamlq-gateway.env")
	os.Remove(path)
}
