package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// style helpers: bold/dim only when writing to a real TTY and NO_COLOR is unset.
func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func bold(w io.Writer, s string) string {
	if !colorEnabled(w) {
		return s
	}
	return "\x1b[1m" + s + "\x1b[0m"
}

func dim(w io.Writer, s string) string {
	if !colorEnabled(w) {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

// drawBox prints a bordered panel. title is shown after "┌─".
func drawBox(w io.Writer, title string, body []string) {
	inner := 2 + visibleLen(strings.TrimSpace(title)) + 2 // " title "
	for _, line := range body {
		if line == "─" {
			continue
		}
		if n := 2 + visibleLen(line); n > inner {
			inner = n
		}
	}
	if inner < 48 {
		inner = 48
	}
	pad := inner - 1 - visibleLen(title)
	if pad < 0 {
		pad = 0
	}
	rule := strings.Repeat("─", inner)
	fmt.Fprintln(w, "┌─"+bold(w, title)+strings.Repeat("─", pad))
	for _, line := range body {
		if line == "─" {
			fmt.Fprintln(w, "├"+rule)
			continue
		}
		fmt.Fprintln(w, "│  "+line)
	}
	fmt.Fprintln(w, "└"+rule)
}
