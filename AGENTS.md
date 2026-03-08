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

## Workspace Isolation (Git Worktrees)

- Every task must run in a dedicated worktree.
- Create worktrees with the repository helper so Beads is bootstrapped automatically:

```bash
./scripts/create_worktree.sh <task-id> <task-branch> [base-ref]
```

- If a worktree was created manually, run this once before `bd ready`:

```bash
./scripts/bootstrap_beads_worktree.sh
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

## Architecture Guidelines

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
