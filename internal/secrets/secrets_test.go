package secrets

import "testing"

func TestHandleRedaction(t *testing.T) {
	h := Handle{Kind: "env", Ref: "OPENROUTER_API_KEY"}
	if s := h.String(); s != "<secret:env:OPENROUTER_API_KEY>" {
		t.Fatalf("String = %q", s)
	}
	if s := h.GoString(); s != h.String() {
		t.Fatalf("GoString leaked: %q", s)
	}
}

func TestParseEnvRef(t *testing.T) {
	h, ok := ParseEnvRef("${ENV:FOO}")
	if !ok || h.Ref != "FOO" {
		t.Fatalf("got %+v ok=%v", h, ok)
	}
	h, ok = ParseEnvRef("env:BAR")
	if !ok || h.Ref != "BAR" {
		t.Fatalf("got %+v ok=%v", h, ok)
	}
	if _, ok := ParseEnvRef("plaintext"); ok {
		t.Fatal("plaintext should not parse as env ref")
	}
}

func TestEnvResolver(t *testing.T) {
	t.Setenv("CG_TEST_SECRET", "super-secret-value")
	r := EnvResolver{}
	v, err := r.Resolve(Handle{Kind: "env", Ref: "CG_TEST_SECRET"})
	if err != nil || v != "super-secret-value" {
		t.Fatalf("resolve: %q %v", v, err)
	}
	if _, err := r.Resolve(Handle{Kind: "env", Ref: "MISSING_CG_SECRET"}); err == nil {
		t.Fatal("expected missing secret error")
	}
}
