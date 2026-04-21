.PHONY: test run bench fmt

test:
	go test ./...

run:
	go run ./cmd/server

bench:
	go test -bench=. -benchmem ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
