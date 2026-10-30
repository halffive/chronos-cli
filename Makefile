STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2026.2.2
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0
GORELEASER := github.com/goreleaser/goreleaser/v2@v2.19.0
SHELL_SCRIPTS := install.sh scripts/*.sh

.PHONY: check fmt-check tidy-check vet lint test vuln no-replace shellcheck release-check snapshot test-install release cover

check: fmt-check tidy-check vet lint test vuln no-replace shellcheck release-check test-install

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
	@if grep -Eq '^[[:space:]]*replace([[:space:]]|\()' go.mod; then echo "go.mod must not contain replace directives"; exit 1; fi

shellcheck:
	./scripts/install-shellcheck.sh dist/tools
	dist/tools/shellcheck $(SHELL_SCRIPTS)

release-check:
	go run $(GORELEASER) check

snapshot:
	go run $(GORELEASER) release --snapshot --clean

test-install: snapshot
	./scripts/test-install.sh

release:
	GORELEASER=$(GORELEASER) ./scripts/release.sh "$$VERSION"

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
