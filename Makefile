.PHONY: fmt test db-up db-down

fmt:
	cd backend && gofmt -w internal/domain

test:
	cd backend && go test ./...

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
