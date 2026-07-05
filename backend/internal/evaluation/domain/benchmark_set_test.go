package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/stretchr/testify/require"
)

func TestBenchmarkSetVersionNumber(t *testing.T) {
	bs, err := domain.NewBenchmarkSet("bs-1", "tenant-1", "owner-1", 5)
	require.NoError(t, err)
	require.Equal(t, 5, bs.VersionNumber())
}

func TestBenchmarkSetTasks(t *testing.T) {
	bs, err := domain.NewBenchmarkSetWithTasks(
		"bs-1", "tenant-1", "owner-1", 1,
		"Benchmark A", "description",
		[]domain.BenchmarkTask{
			{TaskRef: "task-1", Ordering: 0},
			{TaskRef: "task-2", Ordering: 1},
		},
		time.Now(),
	)
	require.NoError(t, err)
	require.Len(t, bs.Tasks(), 2)
	require.Equal(t, "task-1", bs.Tasks()[0].TaskRef)
	require.Equal(t, "task-2", bs.Tasks()[1].TaskRef)
}

func TestNewBenchmarkSetValidatesInputs(t *testing.T) {
	_, err := domain.NewBenchmarkSet("", "tenant-1", "owner-1", 1)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewBenchmarkSet("bs-1", "", "owner-1", 1)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewBenchmarkSet("bs-1", "tenant-1", "owner-1", 0)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
}
