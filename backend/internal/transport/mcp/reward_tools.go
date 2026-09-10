package mcp

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/auth"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RewardService 是 MCP 层消费的奖励服务边界，与 REST 的 RewardService
// 指向同一个 *rewardaccess.Service 实例。
type RewardService interface {
	PublicTaskReward(ctx context.Context, publicTaskID string) (rewardapp.Envelope[rewardapp.PolicyView], error)
	ExecutionReward(ctx context.Context, principal auth.Principal, executionID string) (rewardapp.Envelope[rewardapp.LockView], error)
	ChallengePayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.ChallengePayoutDestination) (rewardapp.Envelope[rewardapp.ChallengeView], error)
	VerifyPayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.VerifyPayoutDestination) (rewardapp.Envelope[rewardapp.DestinationView], error)
}

// RewardGetInput 是 reward_get 工具的输入。二选一：
// public_task_id 查任务级契约（匿名可读），execution_id 查自己那次执行的锁定
// （需要本人授权或任务级 grant）。
type RewardGetInput struct {
	PublicTaskID string `json:"public_task_id,omitempty" jsonschema:"public task identifier; returns the task-level reward policy"`
	ExecutionID  string `json:"execution_id,omitempty" jsonschema:"execution identifier; returns the reward lock for that execution"`
}

// RewardDestinationChallengeInput 请求一次性 nonce。
type RewardDestinationChallengeInput struct {
	RequestID string `json:"request_id" jsonschema:"idempotency key"`
	Chain     string `json:"chain" jsonschema:"settlement chain identifier"`
	Address   string `json:"address" jsonschema:"payout address on that chain"`
}

// RewardDestinationVerifyInput 消费 nonce 完成绑定。
type RewardDestinationVerifyInput struct {
	RequestID string `json:"request_id" jsonschema:"idempotency key"`
	Nonce     string `json:"nonce" jsonschema:"nonce issued by reward_destination_challenge"`
	Chain     string `json:"chain" jsonschema:"settlement chain identifier"`
	Address   string `json:"address" jsonschema:"payout address on that chain"`
}

// registerRewardTools 注册奖励查询与收款目的地绑定工具。
//
// 与文档 §6 的单个 reward_destination_verify 不同，这里拆成 challenge + verify
// 两个工具：nonce 必须由服务端签发且一次性，单个工具做不到防重放。
func registerRewardTools(server *mcp.Server, svc RewardService, principal auth.Principal, idempotency mutationIdempotencyStore) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reward_get",
		Description: "查询公共任务的奖励契约，或某次执行的奖励锁定状态",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RewardGetInput) (*mcp.CallToolResult, any, error) {
		if svc == nil {
			return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "reward ledger is not configured"}), nil, nil
		}
		// 两个参数指向不同的授权模型，同时给出会让"这次调用需要什么权限"
		// 变得含糊，因此直接拒绝。
		if (input.PublicTaskID == "") == (input.ExecutionID == "") {
			return errorResult(MCPError{
				Code:    "INVALID_ARGUMENT",
				Message: "exactly one of public_task_id or execution_id is required",
			}), nil, nil
		}
		if input.PublicTaskID != "" {
			result, err := svc.PublicTaskReward(ctx, input.PublicTaskID)
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		}
		result, err := svc.ExecutionReward(ctx, principal, input.ExecutionID)
		if err != nil {
			return mapDomainError(err, principal), nil, nil
		}
		return successResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reward_destination_challenge",
		Description: "为收款地址申请一次性绑定 nonce",
	}, idempotentMutation(idempotency, "reward_destination_challenge", principal,
		func(input RewardDestinationChallengeInput) string { return input.RequestID },
		func(ctx context.Context, req *mcp.CallToolRequest, input RewardDestinationChallengeInput) (*mcp.CallToolResult, any, error) {
			if svc == nil {
				return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "reward ledger is not configured"}), nil, nil
			}
			result, err := svc.ChallengePayoutDestination(ctx, principal,
				rewardapp.ChallengePayoutDestination{Chain: input.Chain, Address: input.Address})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reward_destination_verify",
		Description: "用 challenge 签发的 nonce 完成收款地址绑定，并撤销旧地址",
	}, idempotentMutation(idempotency, "reward_destination_verify", principal,
		func(input RewardDestinationVerifyInput) string { return input.RequestID },
		func(ctx context.Context, req *mcp.CallToolRequest, input RewardDestinationVerifyInput) (*mcp.CallToolResult, any, error) {
			if svc == nil {
				return errorResult(MCPError{Code: "NOT_IMPLEMENTED", Message: "reward ledger is not configured"}), nil, nil
			}
			result, err := svc.VerifyPayoutDestination(ctx, principal, rewardapp.VerifyPayoutDestination{
				Nonce: input.Nonce, Chain: input.Chain, Address: input.Address,
			})
			if err != nil {
				return mapDomainError(err, principal), nil, nil
			}
			return successResult(result), nil, nil
		}))
}
