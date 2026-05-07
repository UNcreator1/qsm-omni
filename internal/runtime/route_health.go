package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RouteHealthResult struct {
	Model         string    `json:"model"`
	OK            bool      `json:"ok"`
	LatencyMS     int64     `json:"latency_ms"`
	ResponseShape string    `json:"response_shape"`
	ContentOK     bool      `json:"content_ok"`
	Error         string    `json:"error,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

type RouteHealthProbe struct {
	Model   string `json:"model"`
	OK      bool   `json:"ok"`
	Content string `json:"content,omitempty"`
}

type RouterModel struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

func ListRouterModels(ctx context.Context, cfg Config, timeout time.Duration) ([]RouterModel, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	url := strings.TrimRight(cfg.NineRouterURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if cfg.NineRouterKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.NineRouterKey)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var raw struct {
		Data []RouterModel `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw.Data, nil
}

func ProbeRouteHealth(ctx context.Context, cfg Config, models []string, timeout time.Duration) []RouteHealthResult {
	seen := map[string]bool{}
	var out []RouteHealthResult
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, probeOneRoute(ctx, cfg, model, timeout))
	}
	return out
}

func probeOneRoute(ctx context.Context, cfg Config, model string, timeout time.Duration) RouteHealthResult {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	result := RouteHealthResult{
		Model:     model,
		CheckedAt: time.Now().UTC(),
	}
	if cfg.NineRouterURL == "" {
		result.Error = "9Router URL is not configured"
		return result
	}
	if cfg.NineRouterKey == "" {
		result.Error = "9Router API key is not configured"
		return result
	}

	body := map[string]any{
		"model":       model,
		"temperature": 0,
		"max_tokens":  16,
		"stream":      false,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly OK."},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	url := strings.TrimRight(cfg.NineRouterURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.NineRouterKey)

	start := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ResponseShape = "http_error"
		result.Error = fmt.Sprintf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
		return result
	}

	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "data:") {
		result.ResponseShape = "sse_stream"
		result.Error = "route returned SSE chunks for non-stream chat request"
		return result
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content   any    `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		result.ResponseShape = "invalid_json"
		result.Error = err.Error()
		return result
	}
	result.ResponseShape = "openai_chat_json"
	if len(raw.Choices) == 0 {
		result.Error = "chat response had no choices"
		return result
	}
	content := coerceContent(raw.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		if strings.TrimSpace(raw.Choices[0].Message.Reasoning) != "" {
			result.Error = "chat response had reasoning but empty content"
		} else {
			result.Error = "chat response content was empty"
		}
		return result
	}
	result.ContentOK = true
	result.OK = true
	return result
}

func coerceContent(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
