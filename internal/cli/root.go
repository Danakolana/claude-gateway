package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

// Exit codes (stable).
const (
	ExitOK            = 0
	ExitUsage         = 2
	ExitInvalidConfig = 3
	ExitInternal      = 1
)

// Run executes the CLI with the given args (without program name).
func Run(args []string) int {
	return RunWith(args, os.Stdout, os.Stderr, secrets.EnvResolver{})
}

// RunWith is the testable entrypoint.
func RunWith(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return ExitOK
	}

	switch args[0] {
	case "config":
		return runConfig(args[1:], stdout, stderr, resolver)
	case "profile":
		return runProfile(args[1:], stdout, stderr, resolver)
	case "version":
		fmt.Fprintln(stdout, "claude-gateway 0.0.0-dev")
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printHelp(stderr)
		return ExitUsage
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `claude-gateway — Claude Desktop Gateway CLI

Usage:
  claude-gateway <command> [flags]

Commands:
  config validate   Validate configuration
  config explain    Show effective redacted configuration
  profile list      List profiles
  profile create    Create a profile
  profile select    Select active profile
  profile delete    Delete a profile
  version           Print version
  help              Show this help

Exit codes:
  0  success
  1  internal failure
  2  usage / invalid input
  3  invalid configuration`)
}

func runConfig(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: claude-gateway config <validate|explain>")
		return ExitUsage
	}
	switch args[0] {
	case "validate":
		return configValidate(args[1:], stdout, stderr, resolver)
	case "explain":
		return configExplain(args[1:], stdout, stderr, resolver)
	default:
		fmt.Fprintf(stderr, "unknown config subcommand %q\n", args[0])
		return ExitUsage
	}
}

func configValidate(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, err := flagValue(args, "--config")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "load error: %v\n", err)
		return ExitInvalidConfig
	}
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(stderr, e.Error())
		}
		return ExitInvalidConfig
	}
	fmt.Fprintf(stdout, "OK: configuration valid (source=%s, profile=%s)\n", src, cfg.ActiveProfile)
	return ExitOK
}

func configExplain(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, err := flagValue(args, "--config")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "load error: %v\n", err)
		return ExitInvalidConfig
	}
	eff, err := config.Resolve(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "resolve error: %v\n", err)
		return ExitInvalidConfig
	}
	fmt.Fprint(stdout, config.Explain(eff, src))
	return ExitOK
}

func runProfile(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: claude-gateway profile <list|create|select|delete>")
		return ExitUsage
	}
	path, _ := flagValue(args[1:], "--config")
	switch args[0] {
	case "list":
		cfg, _, err := config.Load(path, resolver)
		if err != nil {
			fmt.Fprintf(stderr, "load error: %v\n", err)
			return ExitInvalidConfig
		}
		names := config.ProfileNames(cfg)
		for _, n := range names {
			mark := " "
			if n == cfg.ActiveProfile {
				mark = "*"
			}
			fmt.Fprintf(stdout, "%s %s\n", mark, n)
		}
		return ExitOK
	case "create":
		name, err := flagValue(args[1:], "--name")
		if err != nil || name == "" {
			fmt.Fprintln(stderr, "usage: profile create --name <name> [--config path]")
			return ExitUsage
		}
		if err := config.CreateProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "create error: %v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "created profile %q\n", name)
		return ExitOK
	case "select":
		name, err := flagValue(args[1:], "--name")
		if err != nil || name == "" {
			fmt.Fprintln(stderr, "usage: profile select --name <name> [--config path]")
			return ExitUsage
		}
		if err := config.SelectProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "select error: %v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "selected profile %q\n", name)
		return ExitOK
	case "delete":
		name, err := flagValue(args[1:], "--name")
		if err != nil || name == "" {
			fmt.Fprintln(stderr, "usage: profile delete --name <name> [--config path]")
			return ExitUsage
		}
		if err := config.DeleteProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "delete error: %v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "deleted profile %q\n", name)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown profile subcommand %q\n", args[0])
		return ExitUsage
	}
}

func flagValue(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == name {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), nil
		}
	}
	return "", nil
}
