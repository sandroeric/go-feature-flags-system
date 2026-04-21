.PHONY: test run bench fmt docker-build docker-up docker-down docker-logs

test:
	go test ./...

run:
	go run ./cmd/server

bench:
	go test -bench=. -benchmem ./...

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
