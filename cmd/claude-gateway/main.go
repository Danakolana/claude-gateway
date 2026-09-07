package main

import (
	"os"

	"github.com/danakolana/claude-gateway/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
