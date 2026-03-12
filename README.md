# SyndicateCode

An AI coding CLI with a local control plane for secure, auditable agentic coding assistance.

## Overview

SyndicateCode is an AI-powered coding assistant that runs locally, providing:
- **Secure execution** with sandboxed tool runners and command allowlisting
- **Policy enforcement** via trust tiers and sensitivity levels
- **Approval workflows** for sensitive operations
- **Full audit trails** with event replay capabilities
- **Multi-provider support** (Anthropic, OpenAI)

The control plane is the authoritative component—UI clients remain thin and handle only rendering.

## Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25 |
| Database | SQLite3 (WAL mode) |
| AI Providers | Anthropic SDK, OpenAI SDK |
| WebSocket | nhooyr.io/websocket |
| UUID | github.com/google/uuid |

## Architecture

```
┌─────────────┐
│  TUI Client │  (rendering, user input)
└──────┬──────┘
       ↓
┌──────────────┐
│ Control Plane│  (session, policy, context)
└──────┬───────┘
       ↓
┌──────────────┐
│ Agent Runtime │  (planning, tool selection)
└──────┬───────┘
       ↓
┌──────────────┐
│ Tool Runner  │  (sandbox execution)
└──────┬───────┘
       ↓
┌──────────────┐
│ Local Repo   │  (file operations, git)
└──────────────┘
```

### Core Components

| Component | Package | Responsibility |
|-----------|---------|----------------|
| TUI Client | `pkg/tui/` | Rendering, conversation display, approval prompts |
| Control Plane | `internal/controlplane/` | Session lifecycle, policy enforcement, API server |
| Agent Runtime | `internal/agent/` | Planning loop, tool selection, model invocation |
| Tool System | `internal/tools/` | Tool registry and execution |
| Sandbox | `internal/sandbox/` | Secure command execution with allowlists |
| Audit | `internal/audit/` | Event store, audit trails, replay |
| Policy | `internal/policy/` | Trust tier evaluation, provider routing |

## Project Structure

```
.
├── cmd/
│   ├── server/main.go      # Control plane server
│   └── cli/main.go         # TUI client entry point
├── internal/
│   ├── agent/              # AI agent orchestration
│   ├── audit/              # Event store & audit trails
│   ├── context/            # Context management, token budgeting
│   ├── controlplane/       # HTTP API server
│   ├── mcp/                # MCP protocol loader
│   ├── models/             # AI model abstractions
│   │   └── anthropic/      # Anthropic provider
│   │   └── openai/         # OpenAI provider
│   ├── patch/              # Patch engine
│   ├── policy/             # Policy & routing engine
│   ├── sandbox/            # Command execution sandbox
│   ├── secrets/            # Secret management
│   ├── session/            # Session management
│   ├── state/              # State machine definitions
│   ├── tools/              # Tool registry & executor
│   ├── trust/              # Trust tier definitions
│   └── validation/         # Input validation
├── pkg/
│   ├── api/                # Shared API types
│   └── tui/                # Terminal UI
├── docs/                   # Architecture & API specs
└── syndicatecode.db        # SQLite database
```

## Getting Started

### Prerequisites

- Go 1.25+
- SQLite3

### Build

```bash
go build ./...
```

### Run the Server

```bash
go run ./cmd/server
```

The control plane starts on `http://localhost:7777` by default.

### Run the CLI

```bash
go run ./cmd/cli
```

Or set a custom control plane URL:

```bash
SYNDICATE_CONTROLPLANE_URL=http://localhost:7777 go run ./cmd/cli
```

### Testing

```bash
go test ./...
```

### Linting

```bash
golangci-lint run
```

## Key Features

### Trust Tiers

Configure resource limits based on trust level:
- **tier0**: Most restrictive (64KB output, 30s timeout)
- **tier1**: Standard (256KB output, 120s timeout)
- **tier2**: Elevated (512KB output, 300s timeout)
- **tier3**: Full access

### Approval Workflows

Sensitive operations require approval before execution:
- Proposed → Pending → Approved/Denied → Executed

### Policy Engine

- Provider routing based on trust tier, sensitivity, and task
- Configurable via JSON policy files
- Default provider fallback support

### Event Audit

All state transitions generate events:
- Session start/end
- Turn execution
- Tool invocations
- Approval decisions

Events are stored in SQLite with full replay capability.

### Context Budgeting

Token budget allocation across:
- System prompt
- Repository context
- Conversation history

## Development Workflow

This project uses **Beads** for task tracking:

1. Pick ready work: `bd ready --json`
2. Reserve files: `file_reservation_paths(...)` 
3. Announce start: Send message with `[bd-###]` thread
4. Work and update
5. Complete: `bd close bd-###`

See `AGENTS.md` for full conventions.

## Coding Standards

- **Naming**: PascalCase for types/functions, camelCase for variables
- **Errors**: Sentinel errors as package vars (e.g., `ErrToolNotFound`)
- **Testing**: Table-driven tests with `*_test.go` suffix
- **Packages**: Use `internal/` for private packages
- **Imports**: Standard lib → Third-party → Internal (grouped and sorted)

## API Reference

The control plane exposes REST APIs at `http://localhost:7777/api/v1`:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/sessions` | POST | Create session |
| `/sessions` | GET | List sessions |
| `/sessions/{id}/turns` | POST | Submit turn |
| `/tools` | GET | List tools |
| `/approvals` | GET | List approvals |
| `/policy` | GET | Get policy |

WebSocket streaming for real-time events.

See `docs/ai_cli_control_plane_api_spec.md` for full API documentation.

## Documentation

- [Architecture Checklist](docs/ai_cli_v_1_architecture_checklist.md)
- [API Specification](docs/ai_cli_control_plane_api_spec.md)
- [Event Schema](docs/ai_cli_event_schema.md)
- [Tool Capability Contract](docs/ai_cli_tool_capability_contract.md)
- [Approval State Machine](docs/ai_cli_approval_state_machine.md)

## License

Internal project - all rights reserved.
