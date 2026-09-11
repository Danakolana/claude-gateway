package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpendTripFiresOnceWithoutSecrets(t *testing.T) {
	var hits atomic.Int32
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<16))
		w.WriteHeader(204)
	}))
	defer srv.Close()

	var stderr bytes.Buffer
	trip := newSpendTrip(1.0, srv.URL, &stderr)
	trip.maybe(0.5, 1)
	trip.maybe(1.5, 3)
	trip.maybe(2.0, 4)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hits.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d stderr=%s", hits.Load(), stderr.String())
	}
	if bytes.Contains(body, []byte("sk-")) || bytes.Contains(bytes.ToLower(body), []byte("authorization")) {
		t.Fatalf("alert body leaked secrets: %s", body)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["est_usd"].(float64) < 1 {
		t.Fatalf("%v", payload)
	}
}

func TestSpendAlertTimeoutDoesNotBlock(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(3 * time.Second)
	}))
	defer srv.Close()

	trip := newSpendTrip(0.01, srv.URL, io.Discard)
	begin := time.Now()
	trip.maybe(1, 1)
	if time.Since(begin) > 200*time.Millisecond {
		t.Fatalf("maybe blocked for %s", time.Since(begin))
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never started")
	}
}

func TestPostSpendAlertHonorsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := postSpendAlert(ctx, srv.URL, []byte(`{"text":"x"}`))
	if err == nil {
		t.Fatal("expected timeout")
	}
}
