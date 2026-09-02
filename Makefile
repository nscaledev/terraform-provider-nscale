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
#
# PKG narrows the run to one package, e.g. PKG=./internal/services/reservation/.
testacc:
	TF_ACC=1 go test -v -cover -p 1 -timeout 120m $(or $(PKG),./...)

# testacc-env is the developer-facing wrapper: source NSCALE_* and TF_ACC env
# vars from a local .env (gitignored) and then run testacc. Use this rather
# than testacc directly when running by hand on your laptop.
testacc-env:
	@test -f .env || { echo ".env not found — copy a teammate's or pull from your secret store"; exit 1; }
	@set -a; . ./.env; set +a; $(MAKE) testacc

# testacc-profile runs the suite against one gitignored terraform.<profile>.tfvars.
#
# Profiles exist because no single region can satisfy every test: `staging` uses
# no-glo1, the only region with both storage classes, and `staging-reservation`
# uses stgstack, the only one with usable reservation capacity. A profile omits
# the fixtures its region cannot provide, so those tests skip rather than fail.
#
#   make testacc-profile PROFILE=staging
#   make testacc-profile PROFILE=staging-reservation
#   make testacc-profile PROFILE=staging PKG=./internal/services/reservation/

# Every package's testAccPreCheck requires these four before the provider will
# construct a client, so a profile missing one skips the whole suite rather than
# failing. They are checked up front instead of surfacing as a wall of SKIPs at
# the end of a -timeout 120m run.
#
# The remaining NSCALE_TEST_* fixtures are deliberately absent from some
# profiles — that is what makes their tests skip — so they cannot be checked
# here. What catches a typo or a rename in one of those is tfvars-to-env.sh,
# which exits non-zero on a key it has no mapping for.
testacc_required_env = NSCALE_SERVICE_TOKEN NSCALE_ORGANIZATION_ID NSCALE_REGION_ID NSCALE_PROJECT_ID

testacc-profile:
	@test -n "$(PROFILE)" || { echo "PROFILE is required, e.g. make testacc-profile PROFILE=staging"; exit 1; }
	@test -f terraform.$(PROFILE).tfvars || { echo "terraform.$(PROFILE).tfvars not found — pull it from 1Password"; exit 1; }
	@eval "$$(./scripts/tfvars-to-env.sh terraform.$(PROFILE).tfvars)"; \
		missing=""; \
		for v in $(testacc_required_env); do \
			eval "value=\$$$$v"; \
			test -n "$$value" || missing="$$missing $$v"; \
		done; \
		test -z "$$missing" || { echo "terraform.$(PROFILE).tfvars does not set:$$missing"; exit 1; }; \
		$(MAKE) testacc

.PHONY: fmt lint test schema-check schema-update testacc testacc-env testacc-profile build install generate
