default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

# -race runs the unit suite under Go's data-race detector; it pairs with
# -parallel=10 (parallelism is what surfaces races) and is cheap on the unit
# suite. -coverprofile writes a merged profile for CI to publish; -race forces
# atomic cover mode, which we set explicitly for clarity.
test:
	go test -v -race -covermode=atomic -coverprofile=coverage.out -timeout=120s -parallel=10 ./...
	@go tool cover -func=coverage.out | tail -1

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

.PHONY: fmt lint test testacc testacc-env build install generate
