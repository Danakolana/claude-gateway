package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client pushes/pulls sync objects.
type Client struct {
	BaseURL string
	Token   string
	Tenant  string
	User    string
	HTTP    *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	return c.HTTP
}

// Push sends objects with an idempotency key.
func (c *Client) Push(idem string, objects []map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(map[string]any{"tenant": c.Tenant, "user": c.User, "objects": objects})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/sync/push", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idem)
	res, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("push status %d", res.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out, nil
}

// Pull fetches changes for the tenant.
func (c *Client) Pull(cursor string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/sync/changes?tenant="+c.Tenant+"&cursor="+cursor, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("pull status %d", res.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out, nil
}
