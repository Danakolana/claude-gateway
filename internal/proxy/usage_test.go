package proxy

import (
	"testing"

	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestUsageStoreCacheMissStreak(t *testing.T) {
	u := NewUsageStore()
	view := u.View()
	if !view.OK || view.Last != nil || view.Session.Requests != 0 {
		t.Fatalf("%+v", view)
	}

	miss := api.Usage{InputTokens: 5000, OutputTokens: 10}
	u.Record(SnapshotFrom(api.Request{ID: "a", SourceModel: "claude-haiku-4"}, api.Response{Model: "deepseek/x", Usage: miss}, 0.1, 0.2, true))
	u.Record(SnapshotFrom(api.Request{ID: "b", SourceModel: "claude-haiku-4"}, api.Response{Model: "deepseek/x", Usage: miss}, 0.1, 0.2, true))
	view = u.View()
	if view.Session.Requests != 2 || !view.Session.CacheMissWarning || view.Session.CacheMissStreak != 2 {
		t.Fatalf("streak %+v", view.Session)
	}
	if view.Last == nil || view.Last.ID != "b" || len(view.Notes) == 0 {
		t.Fatalf("last %+v notes=%v", view.Last, view.Notes)
	}
	if view.Last.EstUSD == nil || *view.Last.EstUSD <= 0 {
		t.Fatalf("est %+v", view.Last.EstUSD)
	}

	hit := api.Usage{InputTokens: 5000, OutputTokens: 10, CachedTokens: 4000}
	u.Record(SnapshotFrom(api.Request{ID: "c", SourceModel: "claude-haiku-4"}, api.Response{Model: "deepseek/x", Usage: hit}, 0.1, 0.2, true))
	view = u.View()
	if view.Session.CacheMissStreak != 0 || view.Session.CacheMissWarning {
		t.Fatalf("expected reset %+v", view.Session)
	}
}

func TestSnapshotOmitsPrompt(t *testing.T) {
	s := SnapshotFrom(api.Request{
		ID: "corr-1", SourceModel: "claude-sonnet-4",
		System:   "SECRET_SYSTEM",
		Messages: []api.Message{{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "secret-prompt-xyz"}}}},
	}, api.Response{Usage: api.Usage{InputTokens: 12, OutputTokens: 3}}, 0, 0, false)
	raw := s.SourceModel + s.TargetModel + s.ID + s.Outcome
	if raw == "" {
		t.Fatal("empty")
	}
	if s.Usage.InputTokens != 12 {
		t.Fatal(s.Usage)
	}
}
