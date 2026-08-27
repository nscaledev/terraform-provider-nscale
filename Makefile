default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

# NKS is the one service whose OpenAPI client is generated in-tree rather than
# consumed from nscale-sdk-go — see internal/nks/gen.go for why. nks-spec pulls
# a fresh copy of the canonical spec and regenerates the client; review the
# resulting diff to both openapi.yaml and nks.gen.go before committing.
NKS_SPEC_URL = https://raw.githubusercontent.com/nscaledev/openapi/main/nks-core/main/openapi.yaml

nks-spec:
	curl -fsSL -o internal/nks/openapi.yaml $(NKS_SPEC_URL)
	go generate ./internal/nks/...

fmt:
	gofmt -s -w -e .

# -race runs the unit suite under Go's data-race detector; it pairs with
# -parallel=10 (parallelism is what surfaces races) and is cheap on the unit
# suite. -coverprofile writes a merged profile for CI to publish; -race forces
# atomic cover mode, which we set explicitly for clarity.
test:
	go test -v -race -covermode=atomic -coverprofile=coverage.out -timeout=120s -parallel=10 ./...
	@go tool cover -func=coverage.out | tail -1

# schema-check diffs the provider's live schema against the committed baseline at
# testdata/schema/provider-schema.golden.json and fails on drift. Requires
# terraform and jq on PATH; no credentials needed. Run `make schema-update`
# after an intentional schema change.
schema-check:
	./scripts/check-provider-schema.sh

# schema-update regenerates the committed schema baseline. Run it deliberately
# after an intentional schema change (added/removed/renamed attribute, resource,
# or data source), then commit testdata/schema/provider-schema.golden.json — the
# diff is part of your PR and is what reviewers check.
schema-update:
	./scripts/regenerate-schema.sh

# -p 1 serializes packages: acceptance tests share one project, and the API can
# fail to provision resources (e.g. networks) created concurrently across them.
testacc:
	TF_ACC=1 go test -v -cover -p 1 -timeout 120m ./...

# testacc-env is the developer-facing wrapper: source NSCALE_* and TF_ACC env
# vars from a local .env (gitignored) and then run testacc. Use this rather
# than testacc directly when running by hand on your laptop.
testacc-env:
	@test -f .env || { echo ".env not found — copy a teammate's or pull from your secret store"; exit 1; }
	@set -a; . ./.env; set +a; $(MAKE) testacc

.PHONY: fmt lint test schema-check schema-update testacc testacc-env build install generate nks-spec
