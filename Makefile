.PHONY: verify verify-json ci-local format-check lint test race build schema-generate beads-verify beads-verify-commits beads-verify-pr beads-evidence beads-check-closure go-no-go-report

RANGE ?= origin/master..HEAD
BEAD ?=
MR_TITLE ?=
MR_DESCRIPTION ?=
MR_FILE ?=
STRICT_METADATA ?=
GENERATE_EVIDENCE ?=
CLOSURE_CHECK ?=

VERIFY_FLAGS = --range "$(RANGE)" \
	$(if $(BEAD),--bead "$(BEAD)",) \
	$(if $(MR_TITLE),--title "$(MR_TITLE)",) \
	$(if $(MR_DESCRIPTION),--description "$(MR_DESCRIPTION)",) \
	$(if $(MR_FILE),--mr-file "$(MR_FILE)",) \
	$(if $(STRICT_METADATA),--strict-metadata,) \
	$(if $(GENERATE_EVIDENCE),--generate-evidence,) \
	$(if $(CLOSURE_CHECK),--closure-check,)

verify:
	@go run ./tools/verify $(VERIFY_FLAGS)

verify-json:
	@go run ./tools/verify --json $(VERIFY_FLAGS)

format-check:
	@./.gitlab/scripts/check-gofmt.sh

lint:
	@golangci-lint run

test:
	@go test ./...

race:
	@go test -race ./...

build:
	@go build -o bin/ ./cmd/...
	@go vet ./...

schema-generate: ## Regenerate docs/schema/registry.json — run after any schema change
	@go test ./pkg/api/... -update -run TestGeneratedArtifactMatchesCommittedFile -v

beads-verify:
	@go run ./tools/beads verify --range "$(RANGE)" --strict

beads-verify-commits:
	@go run ./tools/beads verify-commits --range "$(RANGE)" --strict

beads-verify-pr:
	@go run ./tools/beads verify-pr --strict --title "$$CI_MERGE_REQUEST_TITLE" --description "$$CI_MERGE_REQUEST_DESCRIPTION"

beads-evidence:
	@test -n "$(BEAD)" || (echo "BEAD variable is required, e.g. make beads-evidence BEAD=l3d.1" && exit 1)
	@go run ./tools/beads generate-evidence --bead "$(BEAD)" --range "$(RANGE)"

beads-check-closure:
	@test -n "$(BEAD)" || (echo "BEAD variable is required, e.g. make beads-check-closure BEAD=l3d.1" && exit 1)
	@go run ./tools/beads check-closure --bead "$(BEAD)"

ci-local: format-check lint race build beads-verify

go-no-go-report:
	@go run ./tools/gonogo --range "$(RANGE)" $(if $(BEAD),--bead "$(BEAD)",) --output bead-evidence/v1-go-no-go.json
