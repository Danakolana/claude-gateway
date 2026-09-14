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

// The explicit desktop_default must NOT be overridden by the cheapest model:
// an expensive explicitly-defaulted model stays default, and a cheaper model
// in the same tier must not become the family default (which is what caused
// Claude Desktop to route background requests to mercury and swap the model).
func TestDesktopPickerRespectsExplicitDefaultOverCheapest(t *testing.T) {
	models := map[string]config.Model{
		"deepseek": {
			ModelID: "deepseek/deepseek-v4-flash-0731", Enabled: true,
			DesktopID: "claude-haiku-4", DesktopLabel: "DeepSeek V4 Flash",
			DesktopTier: "haiku", DesktopDefault: true,
			InputPrice: 0.14, OutputPrice: 0.28,
		},
		"mercury": {
			ModelID: "inception/mercury-2.5-preview", Enabled: true,
			DesktopID: "claude-haiku-4-2", DesktopLabel: "Mercury 2.5 Preview",
			DesktopTier: "haiku",
			InputPrice:  0.04, OutputPrice: 0.15,
		},
	}
	got := config.DesktopPickerEntries(models)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	// DeepSeek is explicitly defaulted and must stay the haiku-family default,
	// even though mercury is cheaper.
	for _, e := range got {
		if e.DesktopID == "claude-haiku-4" && !e.IsDefault {
			t.Fatalf("explicit default clobbered by cheapest: %#v", got)
		}
		if e.DesktopID == "claude-haiku-4-2" && e.IsDefault {
			t.Fatalf("cheapest model wrongly made family default: %#v", got)
		}
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
