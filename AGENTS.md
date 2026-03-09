# AGENTS.md - SyndicateCode Development Guide

Guidelines for agentic coding agents working on the SyndicateCode AI Coding CLI project.

---

## Build Commands

```bash
# Install dependencies
go mod download

# Build all binaries
go build -o bin/ ./cmd/...

# Run all tests
go test ./...

# Run a single test (use full path)
go test -v ./internal/controlplane/ -run TestSessionCreate

# Run tests matching a pattern
go test -v ./... -run "TestSession|TestPolicy"

# Run with coverage
go test -cover ./...

# Run linter
golangci-lint run

# Format code
go fmt ./...

# Check for vulnerabilities
go vet ./...
```

---

## Code Style Guidelines

### General Principles

1. **Explicit over implicit**: Be clear about intent, avoid clever one-liners
2. **Fail fast**: Return errors immediately rather than ignoring them
3. **Defense in depth**: Validate inputs at system boundaries
4. **Audit everything**: Log events for significant operations

### Naming Conventions

- **Files**: `snake_case.go` (e.g., `session_manager.go`)
- **Types/Interfaces**: `PascalCase` (e.g., `SessionManager`)
- **Functions**: `PascalCase` (e.g., `CreateSession`)
- **Variables**: `camelCase` for private, `PascalCase` for exported
- **Constants**: `PascalCase` (e.g., `MaxRetries = 3`)
- **Acronyms**: Use all caps for 2-letter acronyms (e.g., `HTTPClient`)

### Go Specific

- **Packages**: Short, lowercase, no underscores (e.g., `controlplane`)
- **Imports**: Standard library first, then third-party, then internal
- **Error handling**: Always handle errors explicitly, never use `_` discard
- **Context**: Pass `context.Context` as first parameter to functions that may timeout

```go
func ExecuteTool(ctx context.Context, tool Tool) (*Result, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ...
}
```

### Error Handling

```go
if err != nil {
    return nil, fmt.Errorf("failed to create session %s: %w", sessionID, err)
}

var ErrNotFound = errors.New("resource not found")
```

### Types and Interfaces

- Prefer interfaces for dependencies (easier testing)
- Use concrete types for internal implementation
- Define interfaces where they're used (dependency injection)

```go
type SessionStore interface {
    Get(ctx context.Context, id string) (*Session, error)
    Save(ctx context.Context, session *Session) error
}
```

### Logging

- Use structured logging with levels (debug, info, warn, error)
- Include relevant context, never log secrets

```go
logger.Info("tool execution started",
    "tool_id", toolID,
    "session_id", sessionID,
)
```

### Testing

- Unit tests in `*_test.go` files
- Use table-driven tests for multiple input combinations
- Mock interfaces for external dependencies

### Security

- Never commit secrets: use environment variables
- Validate all inputs at system boundaries
- Sanitize outputs before logging or persisting
- Never concatenate SQL; use parameterized queries

---

## Part 12: Bead-Driven Delivery Enforcement

This section defines the enforcement system that ensures all work is traceable to beads, tested, and verifiable before delivery.

### Bead Identity Model

**Bead ID Format:** `l3d.X` or `bd-X` (e.g., `l3d.1`, `bd-42`)

**Canonical Form:** Always use lowercase `l3d.X` format in commits and branch names.

### Traceability Rules

1. **Commit messages MUST contain bead IDs**
   - Format: `<type>(<scope>): <description> [l3d.X]`
   - Example: `feat(state): add lifecycle transitions [l3d.1]`

2. **Branch names SHOULD contain bead IDs (optional)**
   - Format: `feature/l3d-1-description` or `agent/bd-l3d-1-1`

3. **PR descriptions MUST reference beads**
   - Include bead ID in title or description
   - Link acceptance criteria to bead

### Test Linkage Requirements

**All changed Go source files MUST have corresponding test files.**

For each `*.go` file modified:
- There MUST be a `*_test.go` file in the same package
- Tests MUST verify the changed behavior
- Test names MAY include bead ID: `TestFeature_Bead_l3d_1_Description`

### Evidence Generation

**Every bead MUST have an evidence artifact.**

Evidence is generated automatically by CI but can also be generated locally:

```bash
# Generate evidence for a bead
go run ./tools/beads generate-evidence --bead l3d.1 --range origin/master..HEAD \
  --phase format=pass --phase lint=pass --phase test=pass --phase build=pass \
  --phase bead-verify=pass --phase security=pass

# Verify current changes have proper bead linkage
go run ./tools/beads verify --range origin/master..HEAD --strict

# Verify commits in range have bead references
go run ./tools/beads verify-commits --range HEAD~5..HEAD --strict

# Verify PR metadata includes bead governance sections
go run ./tools/beads verify-pr --strict --title "$CI_MERGE_REQUEST_TITLE" --description "$CI_MERGE_REQUEST_DESCRIPTION"
```

Evidence artifacts are stored in `bead-evidence/` directory as JSON files.

### CI Phase Model

The CI pipeline enforces these phases in order:

1. **format** - gofmt check for all tracked Go files
2. **lint** - Run golangci-lint
3. **test** - Run tests with race detector
4. **build** - Compile all binaries and run `go vet`
5. **bead-verify** - Verify commit traceability, change-to-test linkage, and PR metadata
6. **security** - Run gosec security scan
7. **evidence** - Generate per-bead evidence artifacts and verify closure eligibility

**A merge request MUST pass all phases.**

### Closure Verification

A bead is eligible for closure ONLY when:

1. Evidence artifact exists in `bead-evidence/l3d.X.json`
2. Linked commits for that bead are present in evidence
3. Linked bead-tagged tests are present in evidence
4. Evidence shows passing format, lint, test, build, bead-verify, and security phases
5. `bd show SyndicateCode-l3d.X --json` resolves and the issue status is not already closed

```bash
# Check if bead is eligible for closure
go run ./tools/beads check-closure --bead l3d.1
```

### Local Verification Workflow

Before pushing, run:

```bash
# 1. Run canonical local verifier (human-readable)
make verify RANGE=origin/master..HEAD

# 2. Run canonical local verifier (machine-readable)
make verify-json RANGE=origin/master..HEAD

# 3. Verify bead traceability directly (diagnostics)
go run ./tools/beads verify --range origin/master..HEAD --strict

# 4. Generate evidence for your bead
go run ./tools/beads generate-evidence --bead l3d.X --range origin/master..HEAD \
  --phase format=pass --phase lint=pass --phase test=pass --phase build=pass \
  --phase bead-verify=pass --phase security=pass

# 5. Verify closure eligibility from local evidence
go run ./tools/beads check-closure --bead l3d.X
```

### Prohibited Behaviors

- **NEVER** commit without bead reference in message
- **NEVER** merge without passing bead-verify CI stage
- **NEVER** close bead without evidence artifact
- **NEVER** skip test coverage for changed files
- **NEVER** weaken validation to pass tests

---

## Appendix: Architecture

### Repository Layout

```
/cmd/cli           # CLI entrypoint
/cmd/server        # Control plane server

/internal/controlplane  # Core control plane logic
/internal/session      # Session management
/internal/agent        # Agent runtime
/internal/context      # Context assembly
/internal/policy       # Policy enforcement
/internal/tools        # Tool definitions
/internal/sandbox      # Execution sandbox
/internal/audit       # Event logging
/internal/secrets      # Secret detection/filtering
/internal/models       # Data models

/pkg/tui           # TUI components
/pkg/api           # API definitions
/pkg/types         # Shared types
```

### Key Principles

1. **Control plane is authoritative**: UI is just a client; all logic lives in control plane
2. **Policy enforcement below model**: Don't trust the model to enforce restrictions
3. **Patch-based edits**: All file changes should be diff/patch based for auditability
4. **Session replayability**: All operations must be replayable from event store

---

## Verification

Before marking work complete, run:

```bash
go fmt ./... && golangci-lint run && go test -race ./... && go vet ./...
```

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Dolt-powered version control with native sync
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs via Dolt:

- Each write auto-commits to Dolt history
- Use `bd dolt push`/`bd dolt pull` for remote sync
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

<!-- END BEADS INTEGRATION -->
