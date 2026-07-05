package postgres

import (
	"agentguild.dev/agentguild/backend/internal/application"
	reviewpostgres "agentguild.dev/agentguild/backend/internal/review/postgres"
)

func (tx *Tx) Reviews() application.ReviewRepository {
	return reviewpostgres.NewReviewRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) LineComments() application.LineCommentRepository {
	return reviewpostgres.NewLineCommentRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) Rubrics() application.RubricRepository {
	return reviewpostgres.NewRubricRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) Reviewers() application.ReviewerRepository {
	return reviewpostgres.NewReviewerRepositoryFromTx(tx.tx, tx.Now)
}

var _ application.Tx = (*Tx)(nil)
