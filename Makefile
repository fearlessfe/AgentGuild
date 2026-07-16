.PHONY: build test verify fmt db-up db-down

build:
	cd backend && go build ./...
	cd frontend && npm run build
	cd pi-runner && npm run build

test:
	cd backend && go test -race ./... -count=1
	cd frontend && npm test -- --run
	cd pi-runner && npm test

verify: build test
	cd frontend && npm run test:e2e

fmt:
	cd backend && gofmt -w internal/domain

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
