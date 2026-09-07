package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help") {
		fmt.Fprintln(os.Stdout, `gateway-server — Claude Desktop Gateway history server

Usage:
  gateway-server --help

The history server process is separate from the CLI. Implementation arrives
in Phase 6 (deferred). This entrypoint exists so the binary boundary is
established early.`)
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "gateway-server: not yet implemented (Phase 6 deferred). Use --help.")
	os.Exit(2)
}
