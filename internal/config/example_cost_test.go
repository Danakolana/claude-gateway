package config_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
)

func TestExampleConfigCostKnobs(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	f, err := config.ParseFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(f); len(errs) > 0 {
		t.Fatalf("%v", errs)
	}
	p := f.Providers["openrouter"]
	if p.Sort != "price" || p.RequireParameters == nil || !*p.RequireParameters {
		t.Fatalf("provider prefs: %+v", p)
	}
	if p.Sticky == nil || !*p.Sticky {
		t.Fatalf("sticky should be on: %+v", p)
	}
	r := f.Profiles["cheap"].Routing
	if !r.PreferCheapest || !r.EnsurePromptCache {
		t.Fatalf("routing: %+v", r)
	}
	if r.MaxTokensCap != 32768 {
		t.Fatalf("max_tokens_cap: %+v", r)
	}
	m := f.Models["deepseek_flash"]
	if m.ThinkingPolicy != "force_off" {
		t.Fatalf("model policy: %+v", m)
	}
}
