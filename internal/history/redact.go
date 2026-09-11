package history

import (
	"fmt"
	"regexp"
)

// RedactFunc walks stored payloads. Tests may replace it; restore after.
var RedactFunc = RedactValue

var (
	reAPIKey = regexp.MustCompile(`(?i)sk-[A-Za-z0-9_-]{8,}`)
	reBearer = regexp.MustCompile(`(?i)Bearer [A-Za-z0-9._\-+/=]{8,}`)
)

func redactString(s string) string {
	s = reAPIKey.ReplaceAllString(s, "sk-<redacted>")
	s = reBearer.ReplaceAllString(s, "Bearer <redacted>")
	return s
}

// RedactValue copies v with likely secrets replaced. Imperfect by design.
func RedactValue(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		return redactString(t), nil
	case []byte:
		return []byte(redactString(string(t))), nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			r, err := RedactValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, item := range t {
			r, err := RedactValue(item)
			if err != nil {
				return nil, err
			}
			m, _ := r.(map[string]any)
			out[i] = m
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			r, err := RedactValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

// RedactMessages applies RedactFunc to conversation payloads.
func RedactMessages(messages []map[string]any) ([]map[string]any, error) {
	if messages == nil {
		return nil, nil
	}
	raw, err := RedactFunc(messages)
	if err != nil {
		return nil, err
	}
	out, ok := raw.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("redact: unexpected type %T", raw)
	}
	return out, nil
}
