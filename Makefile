VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# The date of the commit, not of the build, so two builds of one commit are the same (spec 051).
BUILD_DATE ?= $(shell git log -1 --format=%cs 2>/dev/null)
LDFLAGS := -s -w -X github.com/diffluence/diffluence/internal/version.Version=$(VERSION) \
	-X github.com/diffluence/diffluence/internal/version.Date=$(BUILD_DATE)
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.2
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: all build web run test test-race fmt fmt-check vet staticcheck vuln licenses web-check collab-check mutation reproducible fuzz check clean

all: web build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/diffluence ./cmd/diffluence

web:
	cd web && npm ci --no-fund --no-audit && npx vite build

run: build
	./bin/diffluence serve

test:
	go test ./...

test-race:
	# internal/app alone nears Go's default of 10 minutes on a slow runner. A real hang still ends here.
	go test -race -count=1 -timeout=20m ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed on:"; gofmt -l cmd internal; exit 1)

vet:
	go vet ./...

staticcheck:
	go run $(STATICCHECK) ./...

vuln:
	go run $(GOVULNCHECK) ./...

licenses:
	node scripts/check-licenses.mjs

collab-check:
	cd collab && npm ci --no-fund --no-audit && node --test

web-check:
	cd web && npx tsc --noEmit && npx eslint src e2e && npx prettier --check src e2e && npm run -s strings && npm run -s design && npx vitest run

# Do two builds of the binary and the web assets match byte for byte? (spec 057)
reproducible:
	bash scripts/reproducible.sh

# Do the tests catch real bugs? Slow, so not part of every check.
mutation:
	bash scripts/mutation-check.sh

fuzz:
	go test ./internal/document/ -run '^$$' -fuzz FuzzFromMarkdown -fuzztime 60s
	go test ./internal/office/ -run '^$$' -fuzz FuzzReadDOCX -fuzztime 60s
	go test ./internal/exchange/ -run '^$$' -fuzz FuzzFrontMatter -fuzztime 30s
	go test ./internal/exchange/ -run '^$$' -fuzz FuzzVerifyBundle -fuzztime 60s
	go test ./internal/document/ -run '^$$' -fuzz FuzzNormalize -fuzztime 60s
	go test ./internal/exchange/ -run '^$$' -fuzz FuzzHTMLImport -fuzztime 60s
	go test ./internal/scim/ -run '^$$' -fuzz FuzzParseFilter -fuzztime 30s
	go test ./internal/search/ -run '^$$' -fuzz FuzzParseQuery -fuzztime 30s

check: fmt-check vet staticcheck vuln licenses web-check collab-check test-race

clean:
	rm -rf bin dist
