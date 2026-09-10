.PHONY: build test verify fmt db-up db-down test-db-up test-db-down test-db-clean

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

# 后端集成测试共用一个常驻 PostgreSQL 容器（见 internal/testdb）。测试会按需
# 自动启动它，这里只是显式入口。
test-db-up:
	docker run --detach --name agentguild-test-postgres \
		--env POSTGRES_DB=agentguild --env POSTGRES_USER=agentguild \
		--env POSTGRES_PASSWORD=agentguild \
		--publish 127.0.0.1:55432:5432 postgres:18.4 \
		-c max_connections=500 -c fsync=off -c full_page_writes=off \
		-c synchronous_commit=off

# --volumes 不能省：postgres 镜像声明了 VOLUME，漏掉它匿名卷就会变成孤儿。
test-db-down:
	docker rm --force --volumes agentguild-test-postgres

# 回收一次性测试容器（agentguild-test-<纳秒>）遗留的孤儿卷。改用共享容器后
# 不再产生新的，此目标只清理存量。
#
# 这里绝不能用 `docker volume prune`：它会删除本机所有悬空卷，包括其他项目的
# 具名卷和本项目的 agentguild_postgres-data 开发库。只删 64 位十六进制命名的
# 匿名卷——那正是一次性容器留下的形态，具名卷不会匹配。
test-db-clean:
	docker ps -aq --filter 'name=agentguild-test-[0-9]' | xargs -r docker rm --force --volumes
	docker volume ls -qf dangling=true | grep -E '^[0-9a-f]{64}$$' | xargs -r docker volume rm
