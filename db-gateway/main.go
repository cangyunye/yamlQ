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
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		port := serveCmd.Int("port", 0, "listen port (default: random)")
		authToken := serveCmd.String("auth-token", "", "optional auth token for API access")
		showVersion := serveCmd.Bool("version", false, "print version and exit")
		serveCmd.Parse(os.Args[2:])

		if *showVersion {
			fmt.Printf("yamlq/db-gateway %s (commit %s)\n", version, commit)
			os.Exit(0)
		}

		runServe(*port, *authToken)
		return
	}

	showVersion := flag.Bool("version", false, "print version and exit")
	mode := flag.String("mode", "cli", "run mode: cli or service")
	authToken := flag.String("auth-token", "", "optional auth token for API access")
	daemonMode := flag.Bool("daemon", false, "run as daemon (stay resident after client disconnects)")
	portFlag := flag.Int("port", 0, "listen port (default: random)")
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

	addr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
	ln, err := net.Listen("tcp", addr)
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

func runServe(port int, authToken string) {
	cfg := config.Service
	mgr := conn.NewManager(cfg)
	srv := server.New(mgr)
	srv.SetServeMode(true)
	if authToken != "" {
		srv.SetAuthToken(authToken)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("listening on port %d\n", actualPort)
	writeEnvFile(actualPort, authToken, true)
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
