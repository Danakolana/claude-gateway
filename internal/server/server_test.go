package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danakolana/claude-gateway/internal/server"
)

func TestAuthAndPush(t *testing.T) {
	s := server.New(server.Config{Token: "tok", DataDir: t.TempDir()})
	h := s.Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/push", bytes.NewBufferString(`{"tenant":"t","user":"u","objects":[{"id":"1","version":1,"checksum":"abc","body":{}}]}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Idempotency-Key", "k1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatal("health should be open")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/v1/sync/changes?tenant=t", nil)
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	if rr3.Code != 401 {
		t.Fatal(rr3.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
}
