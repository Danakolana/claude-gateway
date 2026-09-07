// Package modelstatus joins configured models with live OpenRouter catalog data.
package modelstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
)

// LiveModel is a snapshot of OpenRouter catalog fields we surface to users.
type LiveModel struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	ContextLength   int     `json:"context_length"`
	InputPerMTok    float64 `json:"input_per_mtok"`
	OutputPerMTok   float64 `json:"output_per_mtok"`
	CodingIndex     float64 `json:"coding_index,omitempty"`
	AgenticIndex    float64 `json:"agentic_index,omitempty"`
	IntelligenceIdx float64 `json:"intelligence_index,omitempty"`
	CodeArenaRank   int     `json:"code_arena_rank,omitempty"`
	HasCodingIndex  bool    `json:"-"`
	HasAgenticIndex bool    `json:"-"`
	HasIntelligence bool    `json:"-"`
	HasCodeArena    bool    `json:"-"`
}

// StatusRow joins a configured gateway model with live OpenRouter data.
type StatusRow struct {
	Key          string    `json:"key"`
	DesktopID    string    `json:"desktop_id"`
	DesktopLabel string    `json:"desktop_label"`
	ModelID      string    `json:"model_id"`
	Enabled      bool      `json:"enabled"`
	Live         LiveModel `json:"live"`
	Found        bool      `json:"found"`
	ConfigInput  float64   `json:"config_input_per_mtok,omitempty"`
	ConfigOutput float64   `json:"config_output_per_mtok,omitempty"`
	ConfigCtx    int       `json:"config_context_limit,omitempty"`
}

type catalogResponse struct {
	Data []catalogModel `json:"data"`
}

type catalogModel struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	ContextLength int            `json:"context_length"`
	Pricing       map[string]any `json:"pricing"`
	TopProvider   *struct {
		ContextLength int `json:"context_length"`
	} `json:"top_provider"`
	Benchmarks *struct {
		ArtificialAnalysis *struct {
			IntelligenceIndex float64 `json:"intelligence_index"`
			CodingIndex       float64 `json:"coding_index"`
			AgenticIndex      float64 `json:"agentic_index"`
		} `json:"artificial_analysis"`
		DesignArena []struct {
			Arena    string  `json:"arena"`
			Category string  `json:"category"`
			Rank     int     `json:"rank"`
			Elo      float64 `json:"elo"`
		} `json:"design_arena"`
	} `json:"benchmarks"`
}

// FetchCatalog loads GET {baseURL}/models (OpenRouter-compatible).
func FetchCatalog(ctx context.Context, baseURL, apiKey string, client *http.Client) (map[string]LiveModel, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("openrouter models: status %d: %s", res.StatusCode, truncate(string(raw), 200))
	}
	var parsed catalogResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make(map[string]LiveModel, len(parsed.Data))
	for _, m := range parsed.Data {
		live := LiveModel{
			ID: m.ID, Name: m.Name, ContextLength: m.ContextLength,
			InputPerMTok: perMTok(m.Pricing, "prompt"), OutputPerMTok: perMTok(m.Pricing, "completion"),
		}
		if live.ContextLength == 0 && m.TopProvider != nil {
			live.ContextLength = m.TopProvider.ContextLength
		}
		if m.Benchmarks != nil && m.Benchmarks.ArtificialAnalysis != nil {
			aa := m.Benchmarks.ArtificialAnalysis
			if aa.CodingIndex > 0 {
				live.CodingIndex = aa.CodingIndex
				live.HasCodingIndex = true
			}
			if aa.AgenticIndex > 0 {
				live.AgenticIndex = aa.AgenticIndex
				live.HasAgenticIndex = true
			}
			if aa.IntelligenceIndex > 0 {
				live.IntelligenceIdx = aa.IntelligenceIndex
				live.HasIntelligence = true
			}
		}
		if m.Benchmarks != nil {
			for _, a := range m.Benchmarks.DesignArena {
				if a.Category == "codecategories" && a.Rank > 0 {
					live.CodeArenaRank = a.Rank
					live.HasCodeArena = true
					break
				}
			}
		}
		out[m.ID] = live
	}
	return out, nil
}

func perMTok(pricing map[string]any, key string) float64 {
	if pricing == nil {
		return 0
	}
	v, ok := pricing[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t * 1e6
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f * 1e6
	case json.Number:
		f, _ := t.Float64()
		return f * 1e6
	default:
		return 0
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// BuildStatusRows joins configured models with a live catalog.
func BuildStatusRows(models map[string]config.Model, live map[string]LiveModel, enabledOnly bool) []StatusRow {
	var keys []string
	for k, m := range models {
		if enabledOnly && !m.Enabled {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]StatusRow, 0, len(keys))
	for _, k := range keys {
		m := models[k]
		row := StatusRow{
			Key: k, DesktopID: m.DesktopID, DesktopLabel: m.DesktopLabel,
			ModelID: m.ModelID, Enabled: m.Enabled,
			ConfigInput: m.InputPrice, ConfigOutput: m.OutputPrice, ConfigCtx: m.ContextLimit,
		}
		if row.DesktopLabel == "" {
			row.DesktopLabel = m.DisplayName
		}
		if lm, ok := live[m.ModelID]; ok {
			row.Live = lm
			row.Found = true
		}
		rows = append(rows, row)
	}
	return rows
}

// FormatTable renders a fixed-width status table.
func FormatTable(rows []StatusRow, fetchedAt time.Time) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("OpenRouter live status @ %s\n", fetchedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("%-18s %-36s %10s %10s %10s %8s %8s %8s\n",
		"KEY", "MODEL_ID", "IN$/M", "OUT$/M", "CTX", "CODE", "AGENT", "ARENA"))
	b.WriteString(strings.Repeat("-", 118) + "\n")
	for _, r := range rows {
		in, out, ctx := "—", "—", "—"
		code, agent, arena := "—", "—", "—"
		if r.Found {
			in = fmt.Sprintf("%.3f", r.Live.InputPerMTok)
			out = fmt.Sprintf("%.3f", r.Live.OutputPerMTok)
			if r.Live.ContextLength > 0 {
				ctx = formatTokens(r.Live.ContextLength)
			}
			if r.Live.HasCodingIndex {
				code = fmt.Sprintf("%.1f", r.Live.CodingIndex)
			}
			if r.Live.HasAgenticIndex {
				agent = fmt.Sprintf("%.1f", r.Live.AgenticIndex)
			}
			if r.Live.HasCodeArena {
				arena = fmt.Sprintf("#%d", r.Live.CodeArenaRank)
			}
		} else {
			in, out = "missing", "missing"
		}
		label := r.Key
		if !r.Enabled {
			label = r.Key + "*"
		}
		b.WriteString(fmt.Sprintf("%-18s %-36s %10s %10s %10s %8s %8s %8s\n",
			trimPad(label, 18), trimPad(r.ModelID, 36), in, out, ctx, code, agent, arena))
		if r.DesktopID != "" || r.DesktopLabel != "" {
			b.WriteString(fmt.Sprintf("  desktop: %s — %s\n", r.DesktopID, r.DesktopLabel))
		}
	}
	b.WriteString("\nCODE=Artificial Analysis coding_index; AGENT=agentic_index; ARENA=design_arena codecategories rank\n")
	b.WriteString("*=disabled in config. Prices are USD per 1M tokens from OpenRouter /models.\n")
	return b.String()
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func trimPad(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
