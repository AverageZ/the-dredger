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

type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "gemma3:4b"
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

// Ping checks if the Ollama server is reachable with a short timeout.
func (o *OllamaClient) Ping() bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(o.baseURL + "/api/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (o *OllamaClient) Summarize(ctx context.Context, title, description, url string) (string, []string, error) {
	prompt := fmt.Sprintf(`You are a bookmark assistant. Given a webpage's title, URL, and description, provide:
1. A concise 2-3 sentence summary of what this page is about and why someone might find it useful.
2. 3-5 relevant tags (single words or short hyphenated phrases, lowercase).

Title: %s
URL: %s
Description: %s

Respond in this exact format:
SUMMARY: <your summary>
TAGS: <tag1>, <tag2>, <tag3>`, title, url, description)

	reqBody := ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", nil, fmt.Errorf("decode ollama response: %w", err)
	}

	summary, tags := parseResponse(ollamaResp.Response)
	return summary, tags, nil
}

func parseResponse(raw string) (string, []string) {
	var summary string
	var tags []string

	// Find SUMMARY: line
	if idx := strings.Index(raw, "SUMMARY:"); idx != -1 {
		rest := raw[idx+len("SUMMARY:"):]
		// Summary ends at TAGS: or end of string
		if tagIdx := strings.Index(rest, "TAGS:"); tagIdx != -1 {
			summary = strings.TrimSpace(rest[:tagIdx])
		} else {
			summary = strings.TrimSpace(rest)
		}
	}

	// Find TAGS: line
	if idx := strings.Index(raw, "TAGS:"); idx != -1 {
		rest := strings.TrimSpace(raw[idx+len("TAGS:"):])
		// Take first line only
		if nlIdx := strings.IndexByte(rest, '\n'); nlIdx != -1 {
			rest = rest[:nlIdx]
		}
		for _, t := range strings.Split(rest, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	return summary, tags
}
