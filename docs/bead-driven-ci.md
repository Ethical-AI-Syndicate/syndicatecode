# Bead-Driven CI Enforcement

This repository uses Beads (`l3d.X` IDs) as delivery traceability anchors. CI enforces that every mergeable change is linked to beads, mapped to tests, and backed by an evidence artifact.

## Canonical Identity Model

- Canonical bead format: `l3d.<number>[.<number>...]` (examples: `l3d.1`, `l3d.10.3`)
- Commit subjects must include a canonical bead ID.
- Merge request title or body must include canonical bead IDs.
- Bead-to-test linkage is enforced through test names containing `Bead_l3d_...`.

Examples:

- `feat(state): add transition guard [l3d.1.4]`
- `func TestTurnGateRejectsConcurrentWrites_Bead_l3d_1_4(t *testing.T)`

## CI Phase Model

`.gitlab-ci.yml` defines the enforcement pipeline in this order:

1. `format` - gofmt check (`./.gitlab/scripts/check-gofmt.sh`)
2. `lint` - `golangci-lint run`
3. `test` - `go test -race ./...`
4. `build` - `go build -o bin/ ./cmd/...` + `go vet ./...`
5. `bead` - commit/PR/change verification (`tools/beads`)
6. `security` - `gosec ./...`
7. `evidence` - evidence generation and closure eligibility checks

## Tooling Entrypoints

- `tools/beads/main.go` is the canonical verification implementation.
- `tools/beads/verify` is a shell wrapper (`go run ./tools/beads ...`).
- `tools/verify/main.go` is the canonical one-command local verifier for humans and automation.
- `Makefile` provides local commands.

Primary commands:

- `make beads-verify RANGE=origin/master..HEAD`
- `make beads-verify-commits RANGE=origin/master..HEAD`
- `make beads-evidence BEAD=l3d.1 RANGE=origin/master..HEAD`
- `make beads-check-closure BEAD=l3d.1`
- `make ci-local`

## Structured Local Verifier

Canonical local command:

- `make verify`

Machine-readable mode:

- `make verify-json`
- `go run ./tools/verify --json`

Supported options:

- `--range origin/master..HEAD`
- `--bead l3d.X`
- `--title "..." --description "..."` or `--mr-file path`
- `--generate-evidence`
- `--closure-check`
- `--strict-metadata`

JSON output is schema-versioned (`schema_version: "1"`) and includes deterministic phase objects with status (`pass|fail|skipped`), executed commands, exit codes, summaries, artifacts, readiness booleans, and CI parity notes.

## Evidence Artifact Contract

Evidence files are generated in `bead-evidence/<bead>.json` and contain:

- bead ID, linked bd issue ID/status, range, generation timestamp, HEAD SHA
- linked commits for that bead
- changed files and changed test files
- linked bead-tagged tests
- CI phase status map
- closure credibility summary with explicit failure reasons

Closure eligibility is blocked when evidence shows any required gap:

- no linked commits
- no changed files
- no linked bead-tagged tests
- any required CI phase not `pass`

`check-closure` also verifies the bead exists in `bd` and is not already closed before reporting closure eligibility.

## TDD Proxy Enforcement

CI cannot prove absolute chronological test-first behavior, so it enforces auditable proxies:

- MR template requires explicit TDD attestations
- changed production Go files require changed test files
- changed tests must include bead-tagged test names for referenced beads

This combination makes untested or weakly-linked changes visible and review-blocking.
