package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/diagnose"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

func runDoctor(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	asJSON := hasFlag(args, "--json")
	promptOnly := hasFlag(args, "--prompt")
	offline := hasFlag(args, "--offline")
	cfg, src, err := config.Load(path, resolver)
	var opts diagnose.Options
	opts.Resolver = resolver
	opts.ConfigPath = src
	opts.ProbeProvider = !offline
	if err != nil {
		opts.ConfigErr = err
	} else {
		opts.Config = cfg
		opts.DesktopTarget = cfg.Client.DesktopTarget()
	}
	rep := diagnose.Collect(opts)
	if saved, perr := diagnose.Persist(rep); perr == nil {
		rep = saved
	}

	switch {
	case asJSON:
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	case promptOnly:
		printSupportPrompt(stdout, rep)
	default:
		fmt.Fprint(stdout, diagnose.FormatHuman(rep))
		fmt.Fprintf(stdout, " - verdict: %s — %s\n", rep.Verdict, rep.Summary)
		if rep.Verdict != diagnose.VerdictReady {
			fmt.Fprintln(stdout, "")
			printSupportPrompt(stdout, rep)
		} else {
			fmt.Fprintln(stdout, dim(stdout, "If chat still fails later, run: claude-gateway doctor --prompt"))
		}
	}
	if err != nil {
		return ExitInvalidConfig
	}
	if cfg != nil {
		if errs := config.Validate(cfg); len(errs) > 0 {
			return ExitInvalidConfig
		}
	}
	return ExitOK
}

func printSupportPrompt(w io.Writer, rep diagnose.Report) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, bold(w, "Copy everything between the lines and send it to whoever gave you this app:"))
	fmt.Fprintln(w, "----- claude-gateway support prompt -----")
	fmt.Fprintln(w, strings.TrimRight(rep.SupportPrompt, "\n"))
	fmt.Fprintln(w, "----- end -----")
	if rep.ReportFile != "" {
		fmt.Fprintf(w, "Also saved to %s\n", rep.ReportFile)
	}
}

func printSetupStatus(w io.Writer, rep diagnose.Report) {
	title := " Setup: " + rep.Verdict + " "
	body := []string{rep.Summary, "─"}
	for _, c := range rep.Checks {
		mark := "●"
		status := c.Status
		switch c.Status {
		case diagnose.StatusOK:
			status = green(w, c.Status)
		case diagnose.StatusWarn:
			status = yellow(w, c.Status)
		case diagnose.StatusFail:
			status = red(w, c.Status)
		}
		line := fmt.Sprintf("%s  %-12s %s  %s", mark, c.ID, status, c.Detail)
		body = append(body, line)
		if c.Fix != "" && c.Status != diagnose.StatusOK {
			body = append(body, "     "+dim(w, c.Fix))
		}
	}
	if rep.ReportFile != "" {
		body = append(body, "─", dim(w, "report "+rep.ReportFile))
	}
	fmt.Fprintln(w, "")
	drawBox(w, title, body)
	fmt.Fprintln(w, "")
	if rep.Verdict != diagnose.VerdictReady {
		printSupportPrompt(w, rep)
		fmt.Fprintln(w, "")
	}
}

func publishStatus(rep diagnose.Report, set func(any)) {
	if set == nil {
		return
	}
	set(rep)
}
