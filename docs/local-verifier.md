# Local Verifier (`tools/verify`)

`tools/verify` is the canonical local verification command for engineers, agents, and release tooling.

## Entrypoints

- Human mode: `make verify`
- JSON mode: `make verify-json`
- Make variables: `RANGE`, `BEAD`, `MR_TITLE`, `MR_DESCRIPTION`, `MR_FILE`, `STRICT_METADATA=1`, `GENERATE_EVIDENCE=1`, `CLOSURE_CHECK=1`
- Direct:
  - `go run ./tools/verify --range origin/master..HEAD`
  - `go run ./tools/verify --json --range origin/master..HEAD`

## Phase Model

1. `repo-safety` - git root/branch/default branch/dirty status
2. `format` - `./.gitlab/scripts/check-gofmt.sh`
3. `lint` - `golangci-lint run`
4. `test` - `go test -race ./...`
5. `build` - `go build -o bin/ ./cmd/... && go vet ./...`
6. `bead-verify` - `tools/beads verify-commits` + `tools/beads verify`
7. `metadata-verify` - `tools/beads verify-pr` (runs when metadata provided)
8. `security` - `gosec ./...` (reported as skipped when tool unavailable)
9. `evidence` - optional `tools/beads generate-evidence`
10. `closure-check` - optional `tools/beads check-closure`

## JSON Contract (schema_version `1`)

Top-level fields:

- `schema_version`
- `ok`
- `summary`
- `repo`
- `range`
- `bead`
- `mode`
- `started_at`
- `finished_at`
- `duration_ms`
- `phases`
- `artifacts`
- `errors`
- `warnings`
- `ci_parity`
- `limitations`
- `ready_for_push`
- `ready_for_review`
- `closure_eligible`
- `local_ci_equivalent_ok`

Each phase includes:

- `name`
- `ok`
- `status` (`pass`, `fail`, `skipped`)
- `required`
- `skipped`
- `duration_ms`
- `command`
- `exit_code`
- `stdout_summary`
- `stderr_summary`
- `artifacts`
- `notes`

## Metadata Inputs

Use one of:

- `--title` and `--description`
- `--mr-file` for body text file

Use `--strict-metadata` to fail when metadata is missing.

## Bead and Closure Modes

- `--bead l3d.X`
- `--generate-evidence --bead l3d.X`
- `--closure-check --bead l3d.X`
