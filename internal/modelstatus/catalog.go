// Package modelstatus joins configured models with live OpenRouter catalog data.
package modelstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/mattn/go-isatty"
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
	DesktopTier  string    `json:"desktop_tier,omitempty"`
	ModelID      string    `json:"model_id"`
	Enabled      bool      `json:"enabled"`
	Default      bool      `json:"default,omitempty"`
	Tools        bool      `json:"tools"`
	Vision       bool      `json:"vision"`
	Reasoning    bool      `json:"reasoning"`
	Notes        string    `json:"notes,omitempty"`
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
			DesktopTier: m.DesktopTier, ModelID: m.ModelID, Enabled: m.Enabled,
			Default: m.DesktopDefault, Tools: m.ToolCalls, Vision: m.Vision,
			Reasoning: m.Reasoning, Notes: m.Notes,
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

// FormatTable renders a bordered live status table (one row per model).
func FormatTable(rows []StatusRow, fetchedAt time.Time) string {
	type line struct {
		label, key, in, out, ctx, code, agent, arena string
	}
	lines := make([]line, 0, len(rows))
	labelW, keyW := len("LABEL"), len("KEY")
	for _, r := range rows {
		label := r.DesktopLabel
		if label == "" {
			label = r.Key
		}
		key := r.Key
		if !r.Enabled {
			key = r.Key + "*"
		}
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
		if n := len(label); n > labelW {
			labelW = n
		}
		if n := len(key); n > keyW {
			keyW = n
		}
		lines = append(lines, line{
			label: label, key: key, in: in, out: out, ctx: ctx,
			code: code, agent: agent, arena: arena,
		})
	}
	const maxLabelW, maxKeyW = 36, 18
	if labelW > maxLabelW {
		labelW = maxLabelW
	}
	if keyW > maxKeyW {
		keyW = maxKeyW
	}

	// │  LABEL  KEY  IN$/M OUT$/M CTX CODE AGENT ARENA
	numW := 1 + 7 + 1 + 7 + 1 + 5 + 1 + 5 + 1 + 5 + 1 + 5 // spaces + cols
	contentW := 2 + labelW + 1 + keyW + numW
	header := fmt.Sprintf("●  OpenRouter · %s · $ / 1M tokens", fetchedAt.Format("15:04Z"))
	foot := "CODE/AGENT = Artificial Analysis · ARENA = Design Arena · * = disabled"
	for _, s := range []string{header, foot} {
		if n := 2 + len(s); n > contentW {
			contentW = n
		}
	}
	rule := strings.Repeat("─", contentW)
	title := " Model status "
	pad := contentW - 1 - len(title)
	if pad < 0 {
		pad = 0
	}

	var b strings.Builder
	b.WriteString("┌─" + title + strings.Repeat("─", pad) + "\n")
	b.WriteString("│  " + header + "\n")
	b.WriteString("├" + rule + "\n")
	b.WriteString(fmt.Sprintf("│  %-*s %-*s %7s %7s %5s %5s %5s %5s\n",
		labelW, "LABEL", keyW, "KEY", "IN$/M", "OUT$/M", "CTX", "CODE", "AGENT", "ARENA"))
	for _, ln := range lines {
		b.WriteString(fmt.Sprintf("│  %-*s %-*s %7s %7s %5s %5s %5s %5s\n",
			labelW, trimPad(ln.label, labelW), keyW, trimPad(ln.key, keyW),
			ln.in, ln.out, ln.ctx, ln.code, ln.agent, ln.arena))
	}
	b.WriteString("├" + rule + "\n")
	b.WriteString("│  " + foot + "\n")
	b.WriteString("└" + rule + "\n")
	return b.String()
}

// PriceBand classifies approximate OpenRouter $/MTok cost for the Desktop picker.
// Uses max(input, output/5) so expensive output models are not labeled "cheap".
func PriceBand(inputPerMTok, outputPerMTok float64) string {
	score := inputPerMTok
	if outputPerMTok/5 > score {
		score = outputPerMTok / 5
	}
	switch {
	case score < 0.5:
		return "cheap"
	case score < 3:
		return "mid"
	default:
		return "pricey"
	}
}

// ProviderDisplayName maps a config provider key to a short picker suffix.
func ProviderDisplayName(providerKey string) string {
	switch strings.ToLower(strings.TrimSpace(providerKey)) {
	case "openrouter":
		return "OpenRouter"
	case "nine_router", "ninerouter", "9router":
		return "9router"
	default:
		return strings.TrimSpace(providerKey)
	}
}

var legacyLabelSuffixes = []string{"(gateway)", "(OpenRouter)", "(9router)", "(openrouter)"}

// StripDesktopLabelNoise removes prior price hints and provider/gateway suffixes
// so labels can be re-annotated cleanly on apply.
func StripDesktopLabelNoise(base string) string {
	base = strings.TrimSpace(base)
	if i := strings.Index(base, " · ~$"); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	for {
		lower := strings.ToLower(base)
		trimmed := false
		for _, suf := range legacyLabelSuffixes {
			ls := strings.ToLower(suf)
			if strings.HasSuffix(lower, ls) {
				base = strings.TrimSpace(base[:len(base)-len(ls)])
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return strings.TrimSpace(base)
}

// WithProviderLabelSuffix returns "Name (OpenRouter)" / "Name (9router)" using
// the active provider. Empty providerKey leaves the cleaned base unchanged.
func WithProviderLabelSuffix(base, providerKey string) string {
	base = StripDesktopLabelNoise(base)
	if base == "" {
		base = "model"
	}
	name := ProviderDisplayName(providerKey)
	if name == "" {
		return base
	}
	return base + " (" + name + ")"
}

// AnnotateDesktopLabel appends an approximate price hint for Claude Desktop's picker.
// Example: "DeepSeek V4 Flash (OpenRouter) · ~$0.14/$0.28 · cheap"
func AnnotateDesktopLabel(base string, inputPerMTok, outputPerMTok float64) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "model"
	}
	// Avoid stacking annotations on re-apply.
	if i := strings.Index(base, " · ~$"); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	band := PriceBand(inputPerMTok, outputPerMTok)
	return fmt.Sprintf("%s · ~$%.2f/$%.2f · %s", base, inputPerMTok, outputPerMTok, band)
}

// FormatCompactSnapshot is a short startup table of live/config prices.
func FormatCompactSnapshot(rows []StatusRow, fetchedAt time.Time, fromLive bool) string {
	type line struct {
		label, inS, outS, ctx, band string
		score                       float64
	}
	lines := make([]line, 0, len(rows))
	labelW := len("LABEL")
	for _, r := range rows {
		if r.DesktopID == "" {
			continue
		}
		label := r.DesktopLabel
		if label == "" {
			label = r.Key
		}
		in, out := r.ConfigInput, r.ConfigOutput
		if r.Found {
			in, out = r.Live.InputPerMTok, r.Live.OutputPerMTok
		}
		band := "n/a"
		inS, outS, ctx := "—", "—", "—"
		score := 1e12
		if in > 0 || out > 0 {
			band = PriceBand(in, out)
			inS = fmt.Sprintf("%.2f", in)
			outS = fmt.Sprintf("%.2f", out)
			score = in
			if out/5 > score {
				score = out / 5
			}
		}
		switch {
		case r.Found && r.Live.ContextLength > 0:
			ctx = formatTokens(r.Live.ContextLength)
		case r.ConfigCtx > 0:
			ctx = formatTokens(r.ConfigCtx)
		}
		if n := len(label); n > labelW {
			labelW = n
		}
		lines = append(lines, line{label: label, inS: inS, outS: outS, ctx: ctx, band: band, score: score})
	}
	const maxLabelW = 44
	if labelW > maxLabelW {
		labelW = maxLabelW
	}
	sort.SliceStable(lines, func(i, j int) bool {
		ri, rj := bandRank(lines[i].band), bandRank(lines[j].band)
		if ri != rj {
			return ri < rj
		}
		if lines[i].score != lines[j].score {
			return lines[i].score < lines[j].score
		}
		return lines[i].label < lines[j].label
	})

	// │  ● BAND  LABEL  IN$/M  OUT$/M  CTX   (band col ~9 runes with icon)
	bandColW := 9
	contentW := 2 + bandColW + 1 + labelW + 1 + 8 + 1 + 8 + 1 + 5
	src := "OpenRouter"
	if !fromLive {
		src = "config.toml (live fetch failed)"
	}
	header := fmt.Sprintf("●  approx. $ / 1M tokens · %s · %s", src, fetchedAt.Format("2006-01-02 15:04Z"))
	legend := "↓ cheap <$0.50 · ~ mid <$3 · ↑ pricey ≥$3"
	for _, s := range []string{header, legend} {
		if n := 2 + len(s); n > contentW {
			contentW = n
		}
	}
	rule := strings.Repeat("─", contentW)
	title := " Prices "
	pad := contentW - 1 - len(title)
	if pad < 0 {
		pad = 0
	}

	var b strings.Builder
	b.WriteString("┌─" + title + strings.Repeat("─", pad) + "\n")
	b.WriteString("│  " + header + "\n")
	b.WriteString("├" + rule + "\n")
	b.WriteString(fmt.Sprintf("│  %-*s %-*s %8s %8s %5s\n", bandColW, "BAND", labelW, "LABEL", "IN$/M", "OUT$/M", "CTX"))
	for _, ln := range lines {
		bandCell := formatBandCell(ln.band)
		padBand := bandColW - visibleWidth(bandCell)
		if padBand < 0 {
			padBand = 0
		}
		b.WriteString(fmt.Sprintf("│  %s%s %-*s %8s %8s %5s\n",
			bandCell, strings.Repeat(" ", padBand),
			labelW, trimPad(ln.label, labelW), ln.inS, ln.outS, ln.ctx))
	}
	b.WriteString("├" + rule + "\n")
	b.WriteString("│  " + legend + "\n")
	b.WriteString("└" + rule + "\n")
	return b.String()
}

func bandRank(band string) int {
	switch band {
	case "cheap":
		return 0
	case "mid":
		return 1
	case "pricey":
		return 2
	default:
		return 3
	}
}

func formatBandCell(band string) string {
	switch band {
	case "cheap":
		return colorize("32", "↓ cheap") // green
	case "mid":
		return colorize("33", "~ mid") // yellow
	case "pricey":
		return colorize("31", "↑ pricey") // red
	default:
		return colorize("2", "○ n/a") // dim
	}
}

func colorize(code, s string) string {
	if !snapshotColorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func snapshotColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// Windows consoles often have empty TERM; rely on isatty there.
	if runtime.GOOS != "windows" {
		term := os.Getenv("TERM")
		if term == "" || term == "dumb" {
			return false
		}
	}
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func visibleWidth(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		n++
	}
	return n
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
