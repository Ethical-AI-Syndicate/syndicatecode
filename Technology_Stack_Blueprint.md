# Technology Stack Blueprint

**Project**: SyndicateCode  
**Generated**: 2026-03-12  
**Analysis Depth**: Comprehensive  
**Categorization**: Technology Type

---

## 1. Core Technology Stack

### Language & Runtime

| Technology | Version | Purpose |
|------------|---------|---------|
| Go | 1.25 | Primary programming language |

### Direct Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| github.com/google/uuid | v1.6.0 | UUID generation for session/turn IDs |
| github.com/mattn/go-sqlite3 | v1.14.34 | SQLite database driver |
| nhooyr.io/websocket | v1.8.17 | WebSocket support for streaming |
| github.com/anthropics/anthropic-sdk-go | v1.26.0 | Anthropic AI API client |
| github.com/openai/openai-go/v3 | v3.26.0 | OpenAI API client |
| github.com/tidwall/gjson | v1.18.0 | JSON parsing utilities |
| golang.org/x/sync | v0.16.0 | Synchronization primitives |

---

## 2. Project Architecture

### Directory Structure

```
/home/mike/SyndicateCode/
├── cmd/
│   ├── server/main.go          # HTTP server entry point
│   └── cli/main.go              # TUI CLI entry point
├── internal/                   # Private application packages
│   ├── agent/                  # AI agent orchestration
│   ├── audit/                  # Event store and audit trails
│   ├── context/                # Context management for AI turns
│   ├── controlplane/           # HTTP API server (main server logic)
│   ├── git/                    # Git integration utilities
│   ├── mcp/                    # MCP (Model Context Protocol) loader
│   ├── models/                 # AI model abstractions
│   │   └── anthropic/          # Anthropic provider implementation
│   │   └── openai/             # OpenAI provider implementation
│   ├── patch/                  # Patch/change engine
│   ├── policy/                 # Policy engine and routing
│   ├── sandbox/                # Command execution sandbox
│   ├── secrets/                # Secret management
│   ├── session/                # Session management
│   ├── state/                  # State machine definitions
│   ├── tools/                  # Tool registry and executor
│   ├── trust/                  # Trust tier management
│   └── validation/             # Input validation
├── pkg/
│   ├── api/                    # Shared API types
│   └── tui/                    # Terminal UI client
└── syndicatecode.db            # SQLite database file
```

---

## 3. Package Organization Patterns

### Internal Packages

The project uses Go's `internal` package convention to prevent external imports:

- **Agent Package** (`internal/agent/`): Handles AI agent orchestration, turn management, event emission
- **Controlplane Package** (`internal/controlplane/`): Main HTTP server with routing, middleware, and API handlers
- **Audit Package** (`internal/audit/`): Event store with SQLite backend, migrations, and audit trail
- **Context Package** (`internal/context/`): Turn management, context retrieval, budget allocation
- **Models Package** (`internal/models/`): AI model abstractions with streaming support
- **Tools Package** (`internal/tools/`): Tool registry and executor for tool calling
- **Sandbox Package** (`internal/sandbox/`): Secure command execution with allowed command lists
- **Policy Package** (`internal/policy/`): Provider routing and policy engine
- **Session Package** (`internal/session/`): Session lifecycle management
- **State Package** (`internal/state/`): State machine definitions with transition validation

---

## 4. API Design Patterns

### HTTP API Structure

The controlplane exposes RESTful endpoints under `/api/v1/`:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/sessions` | POST | Create new session |
| `/api/v1/sessions` | GET | List sessions |
| `/api/v1/sessions/{id}` | GET | Get session details |
| `/api/v1/sessions/{id}/turns` | POST | Create turn |
| `/api/v1/tools` | GET | List available tools |
| `/api/v1/health` | GET | Health check |
| `/api/v1/metrics` | GET | Server metrics |
| `/api/v1/approvals` | GET | List pending approvals |
| `/api/v1/approvals/{id}` | POST | Decide on approval |
| `/api/v1/policy` | GET | Get policy document |

### Request/Response Patterns

- **JSON-only**: All requests and responses use JSON encoding
- **Streaming**: WebSocket connections for event streaming
- **Authentication**: Bearer token via `Authorization` header

---

## 5. Data Access Patterns

### Database: SQLite

**Connection**: `sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")`

**Configuration**:
- WAL journal mode for concurrent access
- 5-second busy timeout
- Single connection pool (`db.SetMaxOpenConns(1)`)
- Foreign keys enabled

### Event Store Pattern

The `audit.EventStore` provides:
- Session event recording
- Turn event recording
- Tool invocation tracking
- Model invocation records
- File mutation tracking
- Artifact storage references

---

## 6. State Management Patterns

### State Machines (internal/state/)

Generic state machine implementation with transition validation:

```go
// Example: Session State Transitions
SessionStateActive -> SessionStateCompleted
SessionStateActive -> SessionStateTerminated
```

**States Defined**:
- Session states: active, completed, terminated
- Turn states: active, awaiting_approval, completed, failed, cancelled
- Tool invocation states: proposed, pending_approval, approved, denied, running, succeeded, failed, cancelled
- Approval states: proposed, pending, approved, denied, executed, cancelled
- Edit states: proposed, validated, approved, applying, applied, failed, rolled_back, rejected

---

## 7. Trust & Security Patterns

### Trust Tiers

Defined tiers: `tier0`, `tier1`, `tier2`, `tier3`

Each tier configures:
- `MaxLoopDepth`: Maximum iteration depth
- `MaxToolCalls`: Maximum tool calls per turn
- `MaxOutputBytes`: Maximum output size
- `TurnTimeout`: Maximum turn duration

### Sandbox Execution

- Allowlisted commands only
- Working directory restrictions (within repo root)
- Configurable timeout per command
- Output size limits

---

## 8. AI Model Integration

### Provider Abstraction

```go
// ContentBlock is a sealed sum type for message content parts.
type ContentBlock interface{ contentBlock() }

// Message is a single turn in a conversation.
type Message struct {
    Role    string // "user" or "assistant"
    Content []ContentBlock
}

// Params configures a single model call.
type Params struct {
    Model     string
    Messages  []Message
    Tools     []Tool
    System    string
    MaxTokens int
}
```

### Streaming Events

```go
type StreamEvent interface{ streamEvent() }
type TextDeltaEvent struct{ Delta string }
type ToolUseStartEvent struct{ ID, Name string }
type ToolInputDeltaEvent struct{ ID, Delta string }
type MessageDeltaEvent struct{ OutputTokens int; StopReason string }
```

---

## 9. Tool System Patterns

### Registry Pattern

```go
type Registry struct {
    mu    sync.RWMutex
    tools map[string]ToolDefinition
}

func (r *Registry) Register(tool ToolDefinition) error
func (r *Registry) Get(name string) (ToolDefinition, bool)
func (r *Registry) List() []ToolDefinition
```

### Tool Execution Flow

1. **Registration**: Tools registered at startup with validation
2. **Proposal**: Agent proposes tool call
3. **Approval**: Optional approval gate for sensitive tools
4. **Execution**: Sandbox executes with allowed commands
5. **Result**: Result returned to model

---

## 10. Coding Conventions

### Naming Conventions

- **Types**: PascalCase (e.g., `SessionManager`, `ToolDefinition`)
- **Functions/Methods**: PascalCase (e.g., `NewManager`, `Create`)
- **Variables/Constants**: camelCase or PascalCase for exported
- **Interfaces**: Often end with `-er` (e.g., `Reader`, `Writer`, `EventEmitter`)
- **Errors**: Sentinel errors as `var` (e.g., `ErrToolNotFound`)

### Error Handling

- Use sentinel errors for known error conditions
- Wrap errors with context using `fmt.Errorf("...: %w", err)`
- Custom error types for domain-specific errors

### Testing Patterns

- Table-driven tests for parameterized cases
- `*_test.go` suffix for test files
- Test cleanup with `t.Cleanup()`
- In-memory databases for testing: `:memory:`

### Package Imports

- Standard library first
- Third-party packages
- Internal packages (gitlab.mikeholownych.com/ai-syndicate/syndicatecode/...)
- Grouped and sorted alphabetically within groups

---

## 11. Implementation Examples

### Creating a New Server

```go
func main() {
    ctx := context.Background()
    cfg := controlplane.DefaultConfig()
    
    server, err := controlplane.NewServer(ctx, cfg)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }
    
    if err := server.ListenAndServe(); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
```

### Tool Registration

```go
func (r *Registry) Register(tool ToolDefinition) error {
    if err := tool.Validate(); err != nil {
        return err
    }
    
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if _, exists := r.tools[tool.Name]; exists {
        return ErrToolAlreadyRegistered
    }
    
    r.tools[tool.Name] = tool
    return nil
}
```

### Session State Transition

```go
func ValidateSessionTransition(from, to SessionState) error {
    return validateTransition(
        from, to,
        sessionTransitions,
        []SessionState{SessionStateCompleted, SessionStateTerminated},
        ErrTerminalSessionState,
        ErrInvalidSessionTransition,
        "session",
    )
}
```

---

## 12. Configuration

### Server Config

```go
type Config struct {
    Addr               string        // Server address (default: ":7777")
    DBPath             string        // Database path (default: "syndicatecode.db")
    APIToken           string        // Authentication token
    ReadTimeout        time.Duration // HTTP read timeout (default: 30s)
    WriteTimeout       time.Duration // HTTP write timeout (default: 30s)
    ProviderPolicyPath string        // Path to provider policy JSON
}
```

### Policy Configuration

Provider policy loaded from JSON with:
- Trust tier restrictions
- Sensitivity level restrictions
- Task-based routing
- Retention class configuration

---

## 13. Development Tooling

### Build Commands

- `go build ./...`: Build all packages
- `go test ./...`: Run all tests
- `go vet ./...`: Run static analysis
- `golangci-lint run`: Run linter (see `.golangci.yml`)

### CI/CD

GitLab CI configuration at `.gitlab-ci.yml`:
- Automated testing
- Linting
- Build verification

---

## 14. Technology Decisions Context

### Why These Technologies

| Decision | Rationale |
|----------|-----------|
| Go | Performance, concurrency, simple deployment |
| SQLite | Zero-configuration, single-file storage, ACID compliant |
| WebSocket | Real-time streaming for agent events |
| Anthropic/OpenAI | Multi-provider support for flexibility |
| Internal packages | Go's package visibility for encapsulation |

### Upgrade Paths

- Go 1.25: Current version - follows latest Go releases
- SQLite via go-sqlite3: Maintained driver with WAL support
- WebSocket: nhooyr maintained for Go 1.21+ compatibility

---

## 15. Integration Points

### MCP (Model Context Protocol)

MCP loader in `internal/mcp/` provides plugin-like extensibility for additional tools and capabilities.

### TUI Client

Terminal UI at `pkg/tui/` connects to controlplane API:
- Interactive session management
- Command-based interface
- Event replay capabilities

---

*This blueprint was auto-generated from codebase analysis.*
