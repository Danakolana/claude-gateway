package config_test

import (
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
)

func TestDesktopPickerEntries(t *testing.T) {
	models := map[string]config.Model{
		"b": {
			ModelID: "x/b", Enabled: true, DesktopID: "claude-sonnet-4",
			DesktopLabel: "B", DesktopTier: "sonnet",
			InputPrice: 1.0, OutputPrice: 2.0,
		},
		"a": {
			ModelID: "x/a", Enabled: true, DesktopID: "claude-haiku-4",
			DesktopLabel: "A", DesktopTier: "haiku", DesktopDefault: true,
			InputPrice: 0.10, OutputPrice: 0.20,
		},
		"a2": {
			ModelID: "x/a2", Enabled: true, DesktopID: "anthropic/claude-haiku-4.5",
			DesktopLabel: "Official", DesktopTier: "haiku",
			InputPrice: 0.25, OutputPrice: 1.25,
		},
		"c": {
			ModelID: "x/c", Enabled: true, DesktopID: "claude-opus-4",
			DesktopLabel: "C", DesktopTier: "opus",
			InputPrice: 5.0, OutputPrice: 25.0,
		},
		"skip": {ModelID: "x/skip", Enabled: true}, // no desktop_id
		"off":  {ModelID: "x/off", Enabled: false, DesktopID: "claude-haiku-4-5"},
	}
	got := config.DesktopPickerEntries(models)
	if len(got) != 4 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	// Cheapest input first: A (0.10), Official (0.25), B (1.0), C (5.0)
	if got[0].DesktopID != "claude-haiku-4" || !got[0].IsDefault {
		t.Fatalf("cheapest haiku should lead and be family default: %#v", got[0])
	}
	if got[1].DesktopID != "anthropic/claude-haiku-4.5" || got[1].IsDefault {
		t.Fatalf("second haiku not default: %#v", got[1])
	}
	if got[2].DesktopID != "claude-sonnet-4" || !got[2].IsDefault {
		t.Fatalf("sonnet: %#v", got[2])
	}
	if got[3].DesktopID != "claude-opus-4" || !got[3].IsDefault {
		t.Fatalf("opus: %#v", got[3])
	}
}

func TestLooksLikeAnthropicModelRoute(t *testing.T) {
	ok := []string{"claude-sonnet-4-5", "anthropic/claude-haiku-4.5", "Claude-Opus-4"}
	bad := []string{"", "deepseek/deepseek-v4-flash-0731", "z-ai/glm-5.3-flash", "openai/gpt-4"}
	for _, s := range ok {
		if !config.LooksLikeAnthropicModelRoute(s) {
			t.Fatalf("want ok: %q", s)
		}
	}
	for _, s := range bad {
		if config.LooksLikeAnthropicModelRoute(s) {
			t.Fatalf("want bad: %q", s)
		}
	}
}
