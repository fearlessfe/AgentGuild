package worker_test

import (
	"context"
	"testing"
	"time"

	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reputationworker "agentguild.dev/agentguild/backend/internal/reputation/worker"
	"github.com/stretchr/testify/require"
)

// fakeFactSource 让测试直接控制事实层的水位。
type fakeFactSource struct {
	watermark int64
	facts     []reputationapp.ContributionFact
	listCalls int
}

func (f *fakeFactSource) ListContributionFacts(context.Context, string) ([]reputationapp.ContributionFact, error) {
	f.listCalls++
	return f.facts, nil
}

func (f *fakeFactSource) LatestEventID(context.Context) (int64, error) { return f.watermark, nil }

type fakeParamsRepository struct{}

func (fakeParamsRepository) Get(_ context.Context, algorithmVersion string) (reputationdomain.Params, error) {
	params := reputationdomain.DefaultParams()
	params.AlgorithmVersion = algorithmVersion
	return params, nil
}

type fakeCardRepository struct {
	stored     []reputationdomain.ScoreCard
	watermark  int64
	replaceOps int
}

func (f *fakeCardRepository) ReplaceAlgorithm(_ context.Context, _ string, cards []reputationdomain.ScoreCard) error {
	f.replaceOps++
	f.stored = cards
	return nil
}

func (f *fakeCardRepository) Get(context.Context, string, reputationdomain.ScoreKey) (*reputationdomain.ScoreCard, error) {
	return nil, reputationdomain.ErrNotFound
}

func (f *fakeCardRepository) ListByAgent(context.Context, string, string) ([]reputationdomain.ScoreCard, error) {
	return f.stored, nil
}

func (f *fakeCardRepository) MaxLatestEventID(context.Context, string) (int64, error) {
	return f.watermark, nil
}

func newWorker(t *testing.T, facts *fakeFactSource, cards *fakeCardRepository) *reputationworker.RebuildWorker {
	t.Helper()
	rebuilder, err := reputationapp.NewRebuilder(facts, fakeParamsRepository{}, cards)
	require.NoError(t, err)
	worker, err := reputationworker.NewRebuildWorker(rebuilder, facts, cards,
		reputationdomain.DefaultAlgorithmVersionV2, nil)
	require.NoError(t, err)
	return worker.WithClock(func() time.Time { return time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC) })
}

func TestRebuildWorkerIsNoOpWithoutNewFacts(t *testing.T) {
	facts := &fakeFactSource{watermark: -1}
	cards := &fakeCardRepository{watermark: -1}
	worker := newWorker(t, facts, cards)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Zero(t, cards.replaceOps, "事实层为空时不该重算")
	require.NoError(t, worker.RunOnce(context.Background()))
	require.Zero(t, cards.replaceOps)
}

func TestRebuildWorkerRebuildsOncePerWatermarkChange(t *testing.T) {
	facts := &fakeFactSource{watermark: 42}
	cards := &fakeCardRepository{watermark: -1}
	worker := newWorker(t, facts, cards)
	ctx := context.Background()

	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 1, cards.replaceOps)

	// 水位不变时后续调用是 no-op，不会反复全量重算。
	require.NoError(t, worker.RunOnce(ctx))
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 1, cards.replaceOps)

	facts.watermark = 43
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, 2, cards.replaceOps)
}

func TestRebuildWorkerRequiresDependencies(t *testing.T) {
	_, err := reputationworker.NewRebuildWorker(nil, nil, nil, "", nil)
	require.Error(t, err)
}
