STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2026.2.2
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0

.PHONY: check fmt-check tidy-check vet lint test vuln no-replace cover

check: fmt-check tidy-check vet lint test vuln no-replace

fmt-check:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

tidy-check:
	go mod tidy -diff

vet:
	go vet ./...

lint:
	go run $(STATICCHECK) ./...

test:
	go test -race ./...

vuln:
	go run $(GOVULNCHECK) ./...

no-replace:
	@if go mod edit -json | grep -q '"Replace"'; then echo "go.mod must not contain replace directives"; exit 1; fi

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
