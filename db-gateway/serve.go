package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"yamlq/db-gateway/config"
	"yamlq/db-gateway/conn"
	"yamlq/db-gateway/server"
)

func init() {
	serveEntry = serve
}

func serve() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	port := serveCmd.Int("port", 0, "listen port (default: random)")
	authToken := serveCmd.String("auth-token", "", "optional auth token for API access")
	connIdleTimeout := serveCmd.Int("conn-idle-timeout", 600, "close connection pools idle longer than this many seconds (0 disables)")
	showVersion := serveCmd.Bool("version", false, "print version and exit")
	serveCmd.Parse(os.Args[2:])

	if *showVersion {
		fmt.Printf("yamlq/db-gateway %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	cfg := config.Service
	mgr := conn.NewManager(cfg)
	srv := server.New(mgr)
	srv.SetServeMode(true)
	if *authToken != "" {
		srv.SetAuthToken(*authToken)
	}
	if *connIdleTimeout > 0 {
		mgr.StartReaper(time.Duration(*connIdleTimeout)*time.Second, 30*time.Second)
	}
	run(mgr, srv, *port, *authToken, true)
}
