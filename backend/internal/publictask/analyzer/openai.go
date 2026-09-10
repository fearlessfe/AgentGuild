package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	publictaskanalysis "agentguild.dev/agentguild/backend/internal/publictask/analysis"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIAnalyzer calls an OpenAI-compatible chat-completions endpoint. The
// API key is retained only in memory and is never included in errors or logs.
type OpenAIAnalyzer struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, model, baseURL string, client *http.Client) (*OpenAIAnalyzer, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("openai API key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOpenAIBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback)) {
		return nil, errors.New("openai base URL must be an HTTPS origin")
	}
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	return &OpenAIAnalyzer{apiKey: apiKey, model: model, baseURL: strings.TrimRight(baseURL, "/"), client: client}, nil
}

type chatCompletionsRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *OpenAIAnalyzer) Analyze(ctx context.Context, input publictaskanalysis.Input) (publictaskanalysis.Result, error) {
	contextJSON, err := json.Marshal(struct {
		Repository  string                          `json:"repository"`
		BaseCommit  string                          `json:"base_commit"`
		IssueURL    string                          `json:"issue_url"`
		IssueNumber int                             `json:"issue_number"`
		Title       string                          `json:"title"`
		Problem     string                          `json:"problem"`
		Files       []publictaskanalysis.SourceFile `json:"files"`
	}{input.Repository, input.BaseCommit, input.IssueURL, input.IssueNumber, input.Title, input.Problem, input.Files})
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("encode analysis context: %w", err)
	}
	body, err := json.Marshal(chatCompletionsRequest{
		Model: a.model,
		Messages: []chatMessage{
			{Role: "system", Content: publictaskanalysis.SystemInstruction},
			{Role: "user", Content: publictaskanalysis.TaskContractInstruction + string(contextJSON)},
		},
	})
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("encode analyzer request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint(), bytes.NewReader(body))
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("create analyzer request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+a.apiKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("call analysis provider: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("read analysis provider response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return publictaskanalysis.Result{}, fmt.Errorf("analysis provider returned HTTP %d", resp.StatusCode)
	}
	var response chatCompletionsResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("decode analysis provider response: %w", err)
	}
	if len(response.Choices) == 0 {
		return publictaskanalysis.Result{}, errors.New("analysis provider returned no choices")
	}
	text, err := responseContent(response.Choices[0].Message.Content)
	if err != nil {
		return publictaskanalysis.Result{}, err
	}
	var result publictaskanalysis.Result
	if err := decodeJSON(text, &result); err != nil {
		return publictaskanalysis.Result{}, err
	}
	result = result.Normalize()
	if err := validateResult(result); err != nil {
		return publictaskanalysis.Result{}, err
	}
	return result, nil
}

func (a *OpenAIAnalyzer) endpoint() string {
	if strings.HasSuffix(a.baseURL, "/chat/completions") {
		return a.baseURL
	}
	if strings.HasSuffix(a.baseURL, "/v1") {
		return a.baseURL + "/chat/completions"
	}
	return a.baseURL + "/v1/chat/completions"
}

func responseContent(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) != "" {
			return text, nil
		}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("decode analyzer message content: %w", err)
	}
	for _, block := range blocks {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("analysis provider returned no text")
	}
	return text, nil
}

var _ publictaskanalysis.Analyzer = (*OpenAIAnalyzer)(nil)
