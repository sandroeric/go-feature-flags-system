POSTGRES_DSN ?= postgres://launchdarkly:launchdarkly@localhost:5432/launchdarkly?sslmode=disable

.PHONY: test test-race test-integration run bench bench-profile loadtest fmt docker-build docker-up docker-down docker-logs

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	DATABASE_URL="$(POSTGRES_DSN)" go test -tags=integration ./internal/db

run:
	go run ./cmd/server

bench:
	go test -bench=. -benchmem ./...

bench-profile:
	go test -run=^$$ -bench=BenchmarkEvaluate$ -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./internal/api

loadtest:
	go run ./cmd/loadtest -base-url=http://localhost:8080 -requests=10000 -concurrency=50

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

docker-build:
	docker compose build

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f api
