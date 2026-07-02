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

	"github.com/shopspring/decimal"
)

// LangfuseConfig 配置 Langfuse 读取模式。
type LangfuseConfig struct {
	BaseURL      string
	PublicKey    string
	SecretKey    string
	Mode         string // "cloud" 或 "self-hosted"
	SupportsCost bool   // 自托管实例是否支持成本读取
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
		endpoint = baseURL + "/api/public/metrics"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return unavailable("langfuse"), nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(p.cfg.PublicKey + ":" + p.cfg.SecretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	q := req.URL.Query()
	q.Set("traceTags", fmt.Sprintf("tenant:%s,task:%s,execution:%s,agent_version:%s", ref.TenantID, ref.TaskID, ref.ExecutionID, ref.AgentVersionID))
	req.URL.RawQuery = q.Encode()

	resp, err := p.client.Do(req)
	if err != nil {
		return unavailable("langfuse"), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unavailable("langfuse"), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return unavailable("langfuse"), nil
	}

	var metrics metricsResponse
	if err := json.Unmarshal(body, &metrics); err != nil {
		return unavailable("langfuse"), nil
	}

	cost, ok := metrics.totalCost()
	if !ok {
		return CostObservation{Coverage: CoveragePartial, Provider: "langfuse", Cursor: metrics.Meta.Cursor}, nil
	}
	return CostObservation{ObservedCost: cost, Coverage: CoverageFull, Provider: "langfuse", Cursor: metrics.Meta.Cursor}, nil
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
	TotalCost *float64 `json:"totalCost"`
}

func (r metricsResponse) totalCost() (decimal.Decimal, bool) {
	for _, d := range r.Data {
		if d.TotalCost != nil {
			return decimal.NewFromFloat(*d.TotalCost), true
		}
	}
	return decimal.Zero, false
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
