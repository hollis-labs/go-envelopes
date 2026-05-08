.PHONY: build test lint vuln tidy

build:
	go build ./...

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; skipping"

vuln:
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed; skipping"

tidy:
	go mod tidy
