package routing

import (
	"context"
	"testing"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/provider/fake"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestCircuitBreakerSkipsPrimaryWhenFallbackExists(t *testing.T) {
	ad := &fake.Adapter{FailIfTarget: "p/primary"}
	eng := &Engine{
		Registry: NewRegistry(map[string]config.Model{
			"primary": {ModelID: "p/primary", Enabled: true, Streaming: true, ToolCalls: true, ContextLimit: 8000},
			"backup":  {ModelID: "p/backup", Enabled: true, Streaming: true, ToolCalls: true, ContextLimit: 8000},
		}),
		Adapter: ad,
		Routing: config.Routing{Rules: []config.Rule{
			{Source: "x", TargetModel: "primary"},
			{Source: "x", TargetModel: "backup"},
		}},
		Breaker: NewBreaker(2, time.Minute),
		Retry:   RetryPolicy{MaxAttempts: 1, Backoff: time.Millisecond},
	}
	req := api.Request{SourceModel: "x", MaxTokens: 16, Requirements: api.Requirements{Tools: true}}
	for i := 0; i < 2; i++ {
		_, _, err := eng.Send(context.Background(), req)
		if err == nil {
			t.Fatal("expected primary failure")
		}
	}
	resp, d, err := eng.Send(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if d.TargetModel != "p/backup" || d.Reason != "circuit_breaker" {
		t.Fatalf("%+v", d)
	}
	if resp.Model != "p/backup" {
		t.Fatalf("resp model %s", resp.Model)
	}
}

func TestCircuitBreakerNilFailsOpen(t *testing.T) {
	var b *Breaker
	b.Fail("x")
	if b.Skip("x") {
		t.Fatal("nil breaker must not skip")
	}
}

func TestCircuitBreakerNilMapsFailOpenThenSkip(t *testing.T) {
	b := &Breaker{Failures: 1, Cooldown: time.Minute}
	if b.Skip("x") {
		t.Fatal("uninitialized maps must not skip")
	}
	b.Fail("x")
	if !b.Skip("x") {
		t.Fatal("expected skip after Fail initialized maps")
	}
	b.OK("x")
	if b.Skip("x") {
		t.Fatal("OK must clear skip")
	}
}

func TestCircuitBreakerNoFallbackStillUsesPrimary(t *testing.T) {
	ad := &fake.Adapter{FailIfTarget: "p/only"}
	eng := &Engine{
		Registry: NewRegistry(map[string]config.Model{
			"only": {ModelID: "p/only", Enabled: true, Streaming: true, ToolCalls: true, ContextLimit: 8000},
		}),
		Adapter: ad,
		Routing: config.Routing{Rules: []config.Rule{{Source: "x", TargetModel: "only"}}},
		Breaker: NewBreaker(1, time.Minute),
		Retry:   RetryPolicy{MaxAttempts: 1, Backoff: time.Millisecond},
	}
	req := api.Request{SourceModel: "x", MaxTokens: 16, Requirements: api.Requirements{Tools: true}}
	_, _, _ = eng.Send(context.Background(), req)
	_, d, err := eng.Send(context.Background(), req)
	if err == nil {
		t.Fatal("only model still down")
	}
	if d.TargetModel != "p/only" {
		t.Fatalf("must fail-open to primary, got %+v", d)
	}
}
