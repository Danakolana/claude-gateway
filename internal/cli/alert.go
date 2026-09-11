package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const spendAlertTimeout = time.Second

type spendTrip struct {
	mu        sync.Mutex
	fired     bool
	threshold float64
	url       string
	stderr    io.Writer
	post      func(ctx context.Context, url string, body []byte) error
}

func newSpendTrip(usd float64, url string, stderr io.Writer) *spendTrip {
	if usd <= 0 || url == "" {
		return nil
	}
	return &spendTrip{threshold: usd, url: url, stderr: stderr, post: postSpendAlert}
}

func (t *spendTrip) maybe(estUSD float64, requests int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.fired || estUSD < t.threshold {
		t.mu.Unlock()
		return
	}
	t.fired = true
	url, thresh := t.url, t.threshold
	post := t.post
	t.mu.Unlock()
	payload, _ := json.Marshal(map[string]any{
		"text":     fmt.Sprintf("claude-gateway session spend crossed $%.2f (est. $%.4f)", thresh, estUSD),
		"est_usd":  estUSD,
		"requests": requests,
	})
	go func() {
		defer func() { _ = recover() }()
		ctx, cancel := context.WithTimeout(context.Background(), spendAlertTimeout)
		defer cancel()
		if post == nil {
			return
		}
		if err := post(ctx, url, payload); err != nil && t.stderr != nil {
			fmt.Fprintf(t.stderr, "WARN: spend alert failed (%v); chat continues\n", err)
		}
	}()
}

func postSpendAlert(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Title", "claude-gateway spend")
	client := &http.Client{Timeout: spendAlertTimeout}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<12))
	if res.StatusCode >= 400 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return nil
}
