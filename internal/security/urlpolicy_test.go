package security

import "testing"

func TestClassifyURLHost(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1":                "loopback",
		"::1":                      "loopback",
		"localhost":                "loopback",
		"169.254.169.254":          "blocked",
		"metadata.google.internal": "blocked",
		"10.0.0.1":                 "private",
		"192.168.1.1":              "private",
		"openrouter.ai":            "ok",
		"8.8.8.8":                  "ok",
	}
	for host, want := range cases {
		if got := ClassifyURLHost(host); got != want {
			t.Fatalf("%s: got %q want %q", host, got, want)
		}
	}
}
