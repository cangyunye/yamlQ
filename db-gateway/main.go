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
	flag.Parse()

	if *showVersion {
		fmt.Printf("yamlq/db-gateway %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

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

	httpServer := &http.Server{Handler: srv.Handler()}

	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	srv.SetShutdownFunc(func() {
		mgr.CloseAll()
		httpServer.Shutdown(context.Background())
		os.Exit(0)
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	mgr.CloseAll()
	httpServer.Shutdown(context.Background())
}
