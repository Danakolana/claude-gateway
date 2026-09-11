package cli

import (
	"bytes"
	"strings"
	"testing"
)

func stubDesktopRunning(t *testing.T, running bool) {
	t.Helper()
	orig := desktopRunning
	desktopRunning = func() bool { return running }
	t.Cleanup(func() { desktopRunning = orig })
}

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

func TestConfirmDesktopClosedInteractive(t *testing.T) {
	stubDesktopRunning(t, true)
	in := forcedInteractive{strings.NewReader("y\n")}
	var out, errb bytes.Buffer
	if code := confirmDesktopClosed(in, &out, &errb, false); code != ExitOK {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Claude Desktop is closed?") {
		t.Fatalf("expected prompt: %s", out.String())
	}
}

func TestConfirmDesktopClosedCancel(t *testing.T) {
	stubDesktopRunning(t, true)
	in := forcedInteractive{strings.NewReader("n\n")}
	var out, errb bytes.Buffer
	if code := confirmDesktopClosed(in, &out, &errb, false); code != ExitUsage {
		t.Fatalf("code=%d want usage", code)
	}
}

func TestConfirmDesktopClosedNonInteractive(t *testing.T) {
	stubDesktopRunning(t, true)
	in := strings.NewReader("")
	var out, errb bytes.Buffer
	if code := confirmDesktopClosed(in, &out, &errb, false); code != ExitOK {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errb.String(), "looks like it is running") {
		t.Fatalf("expected warn: %s", errb.String())
	}
}

func TestConfirmDesktopClosedSkippedWhenNotRunning(t *testing.T) {
	stubDesktopRunning(t, false)
	in := forcedInteractive{strings.NewReader("n\n")}
	var out, errb bytes.Buffer
	if code := confirmDesktopClosed(in, &out, &errb, false); code != ExitOK {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errb.String(), "does not appear to be running") {
		t.Fatalf("expected skip: %s", errb.String())
	}
}

func TestConfirmDesktopClosedYesFlag(t *testing.T) {
	stubDesktopRunning(t, true)
	in := forcedInteractive{strings.NewReader("n\n")}
	var out, errb bytes.Buffer
	if code := confirmDesktopClosed(in, &out, &errb, true); code != ExitOK {
		t.Fatalf("code=%d", code)
	}
}

func TestPickDesktopClient(t *testing.T) {
	c, reason := pickDesktopClient(true, false)
	if c.DesktopTarget() != "3p" || reason == "" {
		t.Fatalf("%+v %s", c, reason)
	}
	c, _ = pickDesktopClient(false, true)
	if !c.IsConsumer() || !c.AllowsExperimental() {
		t.Fatalf("%+v", c)
	}
	c, _ = pickDesktopClient(true, true)
	if c.DesktopTarget() != "3p" {
		t.Fatalf("both exist should prefer 3p: %+v", c)
	}
	c, _ = pickDesktopClient(false, false)
	if c.DesktopTarget() != "3p" {
		t.Fatalf("%+v", c)
	}
}

func TestParseDesktopFlag(t *testing.T) {
	var errb bytes.Buffer
	c, code := parseDesktopFlag("consumer", &errb)
	if code != ExitOK || !c.IsConsumer() {
		t.Fatalf("%+v %d %s", c, code, errb.String())
	}
	_, code = parseDesktopFlag("nope", &errb)
	if code != ExitUsage {
		t.Fatalf("code=%d", code)
	}
}
