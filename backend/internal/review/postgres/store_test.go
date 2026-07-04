package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/review/application"
	"agentguild.dev/agentguild/backend/internal/review/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestTxNowCachesClockTimestamp(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	store := postgres.NewStore(db)
	var first, second, third time.Time
	err := store.WithTx(ctx, func(tx application.Tx) error {
		var err error
		first, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		second, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		third, err = tx.Now(ctx)
		if err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, second, third)
}
