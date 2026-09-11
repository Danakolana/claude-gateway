package config_test

import (
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
)

func TestClientDesktopTarget(t *testing.T) {
	var c config.Client
	if c.IsConsumer() || c.DesktopTarget() != "3p" || c.AllowsExperimental() {
		t.Fatalf("%+v", c)
	}
	c.Desktop = "consumer"
	c.AllowExperimental = true
	if !c.IsConsumer() || c.DesktopTarget() != "consumer" || !c.AllowsExperimental() {
		t.Fatalf("%+v", c)
	}
}
