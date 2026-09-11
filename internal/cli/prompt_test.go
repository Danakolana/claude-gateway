package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptDesktopTargetDefault3P(t *testing.T) {
	in := forcedInteractive{strings.NewReader("\n")}
	var out, errb bytes.Buffer
	c, code := promptDesktopTarget(in, &out, &errb)
	if code != ExitOK {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if c.IsConsumer() || c.DesktopTarget() != "3p" {
		t.Fatalf("%+v", c)
	}
}

func TestPromptDesktopTargetConsumerConfirm(t *testing.T) {
	in := forcedInteractive{strings.NewReader("2\ny\n")}
	var out, errb bytes.Buffer
	c, code := promptDesktopTarget(in, &out, &errb)
	if code != ExitOK {
		t.Fatalf("code=%d err=%s out=%s", code, errb.String(), out.String())
	}
	if !c.IsConsumer() || !c.AllowsExperimental() {
		t.Fatalf("%+v", c)
	}
}

func TestPromptDesktopTargetConsumerCancel(t *testing.T) {
	in := forcedInteractive{strings.NewReader("2\nn\n")}
	var out, errb bytes.Buffer
	_, code := promptDesktopTarget(in, &out, &errb)
	if code != ExitUsage {
		t.Fatalf("code=%d want usage", code)
	}
}

func TestPromptDesktopTargetNonInteractive(t *testing.T) {
	in := strings.NewReader("2\ny\n") // not a TTY → ignore input, default 3p
	var out, errb bytes.Buffer
	c, code := promptDesktopTarget(in, &out, &errb)
	if code != ExitOK || c.DesktopTarget() != "3p" {
		t.Fatalf("code=%d client=%+v err=%s", code, c, errb.String())
	}
	if !strings.Contains(errb.String(), "No TTY") {
		t.Fatalf("expected non-interactive notice: %s", errb.String())
	}
}
