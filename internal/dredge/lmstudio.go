package dredge

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

const lmStudioSystemPrompt = "You are a bookmark assistant. Return only the requested SUMMARY and TAGS fields."

type LMStudioClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewLMStudioClient(baseURL, model string) *LMStudioClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:1234"
	}
	if model == "" {
		model = "google/gemma-4-26b-a4b"
	}
	return &LMStudioClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type lmStudioRequest struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Input        string `json:"input"`
}

type lmStudioResponse struct {
	Output  []json.RawMessage `json:"output"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Content  string `json:"content"`
	Response string `json:"response"`
}

// Ping checks if the LM Studio server is reachable with a short timeout.
func (l *LMStudioClient) Ping() bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(l.baseURL + "/api/v1/models")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (l *LMStudioClient) Summarize(ctx context.Context, title, description, url string, comments []string) (string, []string, error) {
	reqBody := lmStudioRequest{
		Model:        l.model,
		SystemPrompt: lmStudioSystemPrompt,
		Input:        buildSummaryPrompt(title, description, url, comments),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("marshal lmstudio request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/api/v1/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("create lmstudio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("lmstudio request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", nil, fmt.Errorf("lmstudio returned status %d: %s", resp.StatusCode, string(body))
	}

	var lmResp lmStudioResponse
	if err := json.NewDecoder(resp.Body).Decode(&lmResp); err != nil {
		return "", nil, fmt.Errorf("decode lmstudio response: %w", err)
	}

	text := lmResp.Text()
	if strings.TrimSpace(text) == "" {
		return "", nil, fmt.Errorf("lmstudio response did not include text")
	}

	summary, tags := parseResponse(text)
	return summary, tags, nil
}

func (r lmStudioResponse) Text() string {
	for _, output := range r.Output {
		if text := textFromRawMessage(output); text != "" {
			return text
		}
	}
	for _, choice := range r.Choices {
		if choice.Message.Content != "" {
			return choice.Message.Content
		}
		if choice.Text != "" {
			return choice.Text
		}
	}
	if r.Message.Content != "" {
		return r.Message.Content
	}
	if r.Content != "" {
		return r.Content
	}
	return r.Response
}

func textFromRawMessage(raw json.RawMessage) string {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}

	var item struct {
		Content json.RawMessage `json:"content"`
		Text    string          `json:"text"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return ""
	}
	if item.Text != "" {
		return item.Text
	}
	return textFromContent(item.Content)
}

func textFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(p.Text)
		}
		return b.String()
	}

	var nested struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &nested); err == nil {
		return nested.Text
	}

	return ""
}
