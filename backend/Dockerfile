FROM golang:1.26.4-alpine AS build

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/agentguild-api ./cmd/agentguild-api

FROM alpine:3.23 AS backend

RUN apk add --no-cache ca-certificates openssl \
    && addgroup -S agentguild \
    && adduser -S -G agentguild -h /var/lib/agentguild agentguild \
    && install -d -o agentguild -g agentguild -m 0700 /var/lib/agentguild
COPY --from=build /out/agentguild-api /usr/local/bin/agentguild-api
COPY --chmod=0755 deploy/backend-entrypoint.sh /usr/local/bin/backend-entrypoint

USER agentguild
ENTRYPOINT ["/usr/local/bin/backend-entrypoint"]

FROM postgres:18.4-alpine AS migrate

COPY backend/migrations/ /migrations/
COPY --chmod=0755 deploy/migrate.sh /usr/local/bin/migrate

ENTRYPOINT ["/usr/local/bin/migrate"]
