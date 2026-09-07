package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reqID  = regexp.MustCompile(`\b((?:FR|NFR|G|U)-[A-Z0-9]+-\d{3}|FR-[A-Z]+-\d{3}|NFR-\d{3}|G-\d{3}|U-\d{3}|ADR-\d{3}|T\d{3}[a-z]?)\b`)
	taskID = regexp.MustCompile(`(?m)^### (T\d{3}[a-z]?) —`)
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	docs := []string{
		"docs/PROJECT.md", "docs/ARCHITECTURE.md", "docs/DOMAIN.md",
		"docs/DECISIONS.md", "docs/DEVELOPMENT.md", "docs/API.md", "docs/DEPLOYMENT.md",
	}
	requiredHeadings := map[string][]string{
		"docs/PROJECT.md":      {"## Start here", "## Canonical knowledge"},
		"docs/ARCHITECTURE.md": {"## System boundary", "## Trust boundaries"},
		"docs/DOMAIN.md":       {"## Core concepts", "## Invariants"},
		"docs/DECISIONS.md":    {"## ADR-001", "## ADR-011"},
		"docs/DEVELOPMENT.md":  {"## Required checks", "## Task discipline"},
		"docs/API.md":          {"## Contract status", "## Local proxy contract"},
		"docs/DEPLOYMENT.md":   {"## Supported targets", "## Security defaults"},
	}

	var failures []string
	for _, d := range docs {
		body, err := os.ReadFile(filepath.Join(root, d))
		if err != nil {
			failures = append(failures, fmt.Sprintf("missing %s: %v", d, err))
			continue
		}
		s := string(body)
		for _, h := range requiredHeadings[d] {
			if !strings.Contains(s, h) {
				failures = append(failures, fmt.Sprintf("%s missing heading %q", d, h))
			}
		}
	}

	reqBody, err := os.ReadFile(filepath.Join(root, "requirements.md"))
	if err != nil {
		failures = append(failures, err.Error())
	}
	taskBody, err := os.ReadFile(filepath.Join(root, "tasks.md"))
	if err != nil {
		failures = append(failures, err.Error())
	}

	definedTasks := map[string]bool{}
	for _, m := range taskID.FindAllStringSubmatch(string(taskBody), -1) {
		definedTasks[m[1]] = true
	}
	// Cross-check: every Txxx referenced in requirements exists in tasks.
	for _, m := range regexp.MustCompile(`\bT\d{3}[a-z]?\b`).FindAllString(string(reqBody), -1) {
		if !definedTasks[m] && m != "T038" && m != "T067" && m != "T104" && m != "T030" && m != "T070" && m != "T004" {
			// Retasked IDs still appear as historical references in tasks.md itself;
			// requirements matrix may still mention split parents — allow listed parents.
			if !definedTasks[m] {
				// Allow parent IDs that were split if any child exists.
				hasChild := false
				for t := range definedTasks {
					if strings.HasPrefix(t, m) && t != m {
						hasChild = true
						break
					}
				}
				if !hasChild && m != "T001" {
					// only fail if truly missing — T001 etc exist
					if !definedTasks[m] {
						failures = append(failures, fmt.Sprintf("requirements.md references missing task %s", m))
					}
				}
			}
		}
	}

	// Contract fixture presence for 9router (T006).
	contract := filepath.Join(root, "testdata/providers/9router/contract.json")
	raw, err := os.ReadFile(contract)
	if err != nil {
		failures = append(failures, "missing 9router contract: "+err.Error())
	} else {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			failures = append(failures, "invalid 9router contract JSON: "+err.Error())
		} else if obj["openai_compatible"] != true {
			failures = append(failures, "9router contract must set openai_compatible=true")
		}
	}

	// ADR-011 must be Accepted before inbound protocol package is considered ready.
	dec, _ := os.ReadFile(filepath.Join(root, "docs/DECISIONS.md"))
	if !strings.Contains(string(dec), "## ADR-011") || !strings.Contains(string(dec), "**Status:** Accepted") {
		// More precise: check ADR-011 section status
		idx := strings.Index(string(dec), "## ADR-011")
		if idx < 0 {
			failures = append(failures, "ADR-011 missing")
		} else {
			section := string(dec)[idx:]
			if end := strings.Index(section[3:], "## ADR-"); end > 0 {
				section = section[:end+3]
			}
			if !strings.Contains(section, "**Status:** Accepted") {
				failures = append(failures, "ADR-011 must be Accepted before Phase 3")
			}
		}
	}

	_ = reqID
	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "docscheck failed:")
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, " -", f)
		}
		os.Exit(1)
	}
	fmt.Println("docscheck OK")
}
