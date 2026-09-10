// Package analyzer contains optional LLM-backed public-task analyzers.
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

const defaultAnthropicBaseURL = "https://api.anthropic.com"

// AnthropicAnalyzer asks a Claude model for a JSON task contract. The API key
// is only held in memory and is never included in errors or logs.
type AnthropicAnalyzer struct {
	apiKey    string
	model     string
	baseURL   string
	client    *http.Client
	maxTokens int
}

func NewAnthropic(apiKey, model, baseURL string, client *http.Client) (*AnthropicAnalyzer, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("anthropic API key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultAnthropicBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback)) {
		return nil, errors.New("anthropic base URL must be an HTTPS origin")
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &AnthropicAnalyzer{apiKey: apiKey, model: model, baseURL: strings.TrimRight(baseURL, "/"), client: client, maxTokens: 1800}, nil
}

type messagesRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system"`
	Messages  []messageRequest `json:"messages"`
}

type messageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *AnthropicAnalyzer) Analyze(ctx context.Context, input publictaskanalysis.Input) (publictaskanalysis.Result, error) {
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
	body, err := json.Marshal(messagesRequest{
		Model: a.model, MaxTokens: a.maxTokens,
		System:   publictaskanalysis.SystemInstruction,
		Messages: []messageRequest{{Role: "user", Content: publictaskanalysis.TaskContractInstruction + string(contextJSON)}},
	})
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("encode analyzer request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("create analyzer request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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
	var response messagesResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return publictaskanalysis.Result{}, fmt.Errorf("decode analysis provider response: %w", err)
	}
	text := ""
	for _, block := range response.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if strings.TrimSpace(text) == "" {
		return publictaskanalysis.Result{}, errors.New("analysis provider returned no text")
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

func decodeJSON(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		if newline := strings.IndexByte(raw, '\n'); newline >= 0 {
			raw = raw[newline+1:]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	start, end := strings.IndexByte(raw, '{'), strings.LastIndexByte(raw, '}')
	if start < 0 || end <= start {
		return errors.New("analysis provider did not return a JSON object")
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), out); err != nil {
		return fmt.Errorf("decode analyzer JSON: %w", err)
	}
	return nil
}

func validateResult(result publictaskanalysis.Result) error {
	for name, value := range map[string]string{
		"title": result.Title, "summary": result.Summary, "problem_diagnosis": result.ProblemDiagnosis,
		"impact": result.Impact, "proposed_solution": result.ProposedSolution,
	} {
		if value == "" {
			return fmt.Errorf("analyzer result missing %s", name)
		}
		if len([]rune(value)) > 4000 {
			return fmt.Errorf("analyzer result %s is too long", name)
		}
	}
	if len(result.ImplementationSteps) == 0 || len(result.AcceptanceCriteria) == 0 {
		return errors.New("analyzer result needs implementation steps and acceptance criteria")
	}
	if len(result.ImplementationSteps) > 12 || len(result.AcceptanceCriteria) > 12 {
		return errors.New("analyzer result contains too many items")
	}
	for _, criterion := range result.AcceptanceCriteria {
		if criterion.ID == "" || criterion.Statement == "" || criterion.VerifierKind == "" || criterion.ExpectedResult == "" {
			return errors.New("analyzer result contains an incomplete acceptance criterion")
		}
		if len([]rune(criterion.Statement)) > 2000 || len([]rune(criterion.ExpectedResult)) > 2000 {
			return errors.New("analyzer result acceptance criterion is too long")
		}
	}
	for _, values := range [][]string{result.ImplementationSteps, result.Constraints, result.NonGoals, result.Risks} {
		for _, value := range values {
			if len([]rune(value)) > 1000 {
				return errors.New("analyzer result list item is too long")
			}
		}
	}
	return nil
}

var _ publictaskanalysis.Analyzer = (*AnthropicAnalyzer)(nil)
