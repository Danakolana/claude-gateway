package api

import "testing"

func TestCapabilitiesUnknownNeverSupported(t *testing.T) {
	c := Capabilities{Tools: CapUnknown, Streaming: CapSupported}
	if c.Compatible(Requirements{Tools: true}) {
		t.Fatal("unknown must not satisfy required tools")
	}
	if !c.Compatible(Requirements{Streaming: true}) {
		t.Fatal("supported streaming should pass")
	}
}

func TestContextOverflow(t *testing.T) {
	c := Capabilities{Streaming: CapSupported, Context: 4096}
	if c.Compatible(Requirements{MinContext: 8000}) {
		t.Fatal("should fail min context")
	}
}

func TestEventTerminal(t *testing.T) {
	if !(Event{Type: EventFinish}).Terminal() {
		t.Fatal("finish is terminal")
	}
	if (Event{Type: EventTextDelta}).Terminal() {
		t.Fatal("delta is not terminal")
	}
}
