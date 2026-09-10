package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// RebuildWorker 按事实层的水位增量触发 v2 声望重算。
//
// 它与 v1 的 Worker 并存：v1 按 review 结论增量更新 reputation_projections，
// v2 从不可变事实全量重算 agent_reputation_projections，两者互不影响。
//
// 每次重算都在**当次调用取得的固定时刻**上进行，并把该时刻写进投影。
// 引入 180 天衰减后，"可完整重算"只在固定评估时刻成立。
type RebuildWorker struct {
	rebuilder        *reputationapp.Rebuilder
	facts            reputationapp.FactSource
	cards            reputationapp.ScoreCardRepository
	algorithmVersion string
	now              func() time.Time
	logger           *slog.Logger

	mu sync.Mutex
	// lastWatermark 记录上一次成功重算时的事实水位。unknownWatermark
	// 表示本进程还没算过，需要先与已落库投影比对。
	lastWatermark int64
}

// unknownWatermark 与任何真实水位都不相等，包括"事实层为空"的 -1。
const unknownWatermark int64 = -2

func NewRebuildWorker(
	rebuilder *reputationapp.Rebuilder,
	facts reputationapp.FactSource,
	cards reputationapp.ScoreCardRepository,
	algorithmVersion string,
	logger *slog.Logger,
) (*RebuildWorker, error) {
	if rebuilder == nil || facts == nil || cards == nil || algorithmVersion == "" {
		return nil, reputationdomain.ErrInvalidArgument
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RebuildWorker{
		rebuilder:        rebuilder,
		facts:            facts,
		cards:            cards,
		algorithmVersion: algorithmVersion,
		now:              func() time.Time { return time.Now().UTC() },
		logger:           logger,
		lastWatermark:    unknownWatermark,
	}, nil
}

// WithClock 让测试固定评估时刻，从而断言重算结果逐字节复现。
func (w *RebuildWorker) WithClock(now func() time.Time) *RebuildWorker {
	if now != nil {
		w.now = now
	}
	return w
}

// RunOnce 在事实层出现新事实时重算一次，否则是 no-op。
func (w *RebuildWorker) RunOnce(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	watermark, err := w.facts.LatestEventID(ctx)
	if err != nil {
		return err
	}
	if watermark == w.lastWatermark {
		return nil
	}
	if w.lastWatermark == unknownWatermark {
		// 冷启动：从未落过投影时必须先算一次；已有投影时也只多算一次，
		// 之后就完全走内存水位。
		stored, err := w.cards.MaxLatestEventID(ctx, w.algorithmVersion)
		if err != nil {
			return err
		}
		if stored == -1 && watermark == -1 {
			w.lastWatermark = watermark
			return nil
		}
	}

	result, err := w.rebuilder.Rebuild(ctx, w.algorithmVersion, w.now())
	if err != nil {
		return err
	}
	w.lastWatermark = watermark
	w.logger.Debug("reputation v2 rebuilt",
		"algorithm_version", result.AlgorithmVersion,
		"evaluated_at", result.EvaluatedAt,
		"projections", result.ProjectionCount,
		"facts", result.FactCount,
	)
	return nil
}
