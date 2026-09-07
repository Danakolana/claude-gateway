package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/danakolana/claude-gateway/internal/server"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Fprintln(os.Stdout, `gateway-server — Claude Desktop Gateway history server

Usage:
  gateway-server [--addr HOST:PORT] [--token TOKEN] [--data DIR]

Environment:
  CLAUDE_GATEWAY_SYNC_TOKEN  bearer token (required unless --token)
  CLAUDE_GATEWAY_DATA_DIR    data directory`)
		os.Exit(0)
	}
	addr := envOr("CLAUDE_GATEWAY_ADDR", "127.0.0.1:8090")
	token := os.Getenv("CLAUDE_GATEWAY_SYNC_TOKEN")
	data := envOr("CLAUDE_GATEWAY_DATA_DIR", "./.gateway-server-data")
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--addr":
			i++
			addr = os.Args[i]
		case "--token":
			i++
			token = os.Args[i]
		case "--data":
			i++
			data = os.Args[i]
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "CLAUDE_GATEWAY_SYNC_TOKEN or --token required")
		os.Exit(2)
	}
	srv := server.New(server.Config{Addr: addr, Token: token, DataDir: data})
	fmt.Println("gateway-server listening on", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
