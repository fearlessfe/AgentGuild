// Langfuse 成本观测适配器。
package telemetry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// LangfuseConfig 配置 Langfuse 读取模式。
type LangfuseConfig struct {
	BaseURL      string
	PublicKey    string
	SecretKey    string
	Mode         string // "cloud" 或 "self-hosted"
	SupportsCost bool   // 自托管实例是否支持成本读取
	MetricsPath  string // 自托管实例兼容的 Metrics API 路径
}

// NewLangfuseProvider 创建 Langfuse TraceCostProvider。
// 当 client 为 nil 时使用默认 HTTP 客户端。
func NewLangfuseProvider(cfg LangfuseConfig, client *http.Client) TraceCostProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &langfuseProvider{cfg: cfg, client: client}
}

type langfuseProvider struct {
	cfg    LangfuseConfig
	client *http.Client
}

func (p *langfuseProvider) Observe(ctx context.Context, ref ExecutionRef) (CostObservation, error) {
	if p.cfg.Mode == "self-hosted" && !p.cfg.SupportsCost {
		return unavailable("langfuse"), nil
	}

	baseURL := strings.TrimRight(p.cfg.BaseURL, "/")
	endpoint := baseURL + "/api/public/v2/metrics"
	if p.cfg.Mode == "self-hosted" {
		// 自托管模式使用兼容 API 路径；若实例不支持成本读取已在上面返回 unavailable。
		path := p.cfg.MetricsPath
		if path == "" {
			path = "/api/public/metrics"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		endpoint = baseURL + path
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return unavailable("langfuse"), nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(p.cfg.PublicKey + ":" + p.cfg.SecretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	query := metricsQuery{
		View:          "observations",
		Dimensions:    []metricDimension{},
		Metrics:       []metric{{Measure: "totalCost", Aggregation: "sum"}},
		FromTimestamp: time.Unix(0, 0).UTC().Format(time.RFC3339),
		ToTimestamp:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
		Filters: []metricFilter{{
			Column: "traceTags", Operator: "all of", Type: "arrayOptions",
			Value: []string{"tenant:" + ref.TenantID, "task:" + ref.TaskID, "execution:" + ref.ExecutionID, "agent_version:" + ref.AgentVersionID},
		}},
	}
	encodedQuery, err := json.Marshal(query)
	if err != nil {
		return unavailable("langfuse"), nil
	}
	q := req.URL.Query()
	q.Set("query", string(encodedQuery))
	req.URL.RawQuery = q.Encode()

	resp, err := p.client.Do(req)
	if err != nil {
		return unavailable("langfuse"), fmt.Errorf("langfuse metrics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unavailable("langfuse"), fmt.Errorf("langfuse metrics status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return unavailable("langfuse"), fmt.Errorf("read langfuse metrics: %w", err)
	}

	var metrics metricsResponse
	if err := json.Unmarshal(body, &metrics); err != nil {
		return unavailable("langfuse"), fmt.Errorf("decode langfuse metrics: %w", err)
	}

	cost, ok, err := metrics.totalCost()
	if err != nil {
		return unavailable("langfuse"), err
	}
	if !ok {
		return CostObservation{Coverage: CoveragePartial, Provider: "langfuse", Cursor: metrics.Meta.Cursor}, nil
	}
	return CostObservation{ObservedCost: cost, Coverage: CoverageComplete, Provider: "langfuse", Cursor: metrics.Meta.Cursor}, nil
}

func unavailable(provider string) CostObservation {
	return CostObservation{Coverage: CoverageUnavailable, Provider: provider, Cursor: ""}
}

type metricsResponse struct {
	Data []metricItem `json:"data"`
	Meta struct {
		Cursor string `json:"cursor"`
	} `json:"meta"`
}

type metricItem struct {
	TotalCost    json.Number `json:"totalCost"`
	SumTotalCost json.Number `json:"sum_totalCost"`
}

func (r metricsResponse) totalCost() (decimal.Decimal, bool, error) {
	for _, d := range r.Data {
		value := d.SumTotalCost
		if value == "" {
			value = d.TotalCost
		}
		if value != "" {
			cost, err := decimal.NewFromString(value.String())
			if err != nil {
				return decimal.Zero, false, fmt.Errorf("decode langfuse total cost: %w", err)
			}
			return cost, true, nil
		}
	}
	return decimal.Zero, false, nil
}

type metricsQuery struct {
	View          string            `json:"view"`
	Dimensions    []metricDimension `json:"dimensions"`
	Metrics       []metric          `json:"metrics"`
	Filters       []metricFilter    `json:"filters"`
	FromTimestamp string            `json:"fromTimestamp"`
	ToTimestamp   string            `json:"toTimestamp"`
}

type metricDimension struct {
	Field string `json:"field"`
}

type metric struct {
	Measure     string `json:"measure"`
	Aggregation string `json:"aggregation"`
}

type metricFilter struct {
	Column   string   `json:"column"`
	Operator string   `json:"operator"`
	Value    []string `json:"value"`
	Type     string   `json:"type"`
}

// FailingProvider 返回一个总是失败的 Provider，用于故障场景测试。
func FailingProvider(err error) TraceCostProvider {
	return &failingProvider{err: err}
}

type failingProvider struct {
	err error
}

func (p *failingProvider) Observe(_ context.Context, _ ExecutionRef) (CostObservation, error) {
	return CostObservation{}, p.err
}
