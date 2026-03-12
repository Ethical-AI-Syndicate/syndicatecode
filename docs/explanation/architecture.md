# Architecture

This document explains how SyndicateCode works at a system level.

## Overview

SyndicateCode is an AI coding assistant with a local control plane. It provides secure, auditable agentic coding assistance by separating the UI from the core logic.

## Core Principle

> The UI is not the product. The product is the control plane, the provenance model, and the safety boundaries. Everything else is an interface to those systems.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        TUI Client                           │
│  (rendering, user input, approval display)                  │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP/WebSocket
┌─────────────────────────▼───────────────────────────────────┐
│                     Control Plane                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │  Session    │  │  Context    │  │  Policy             │ │
│  │  Manager    │  │  Manager    │  │  Engine             │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │  Approval  │  │   Audit     │  │   API               │ │
│  │  Manager   │  │   Store     │  │   Server            │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                    Agent Runtime                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │  Planner    │  │   Model    │  │  Tool               │ │
│  │  Loop       │  │   Client   │  │  Selector           │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                    Tool Layer                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │   Read      │  │   Write     │  │  Execute           │ │
│  │   Tools     │  │   Tools     │  │  Tools             │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                  Local Repository                          │
│  (files, git, shell)                                       │
└─────────────────────────────────────────────────────────────┘
```

## Components

### TUI Client

A thin client that handles:
- Rendering conversation
- Displaying tool execution traces
- Showing approval prompts
- Presenting diffs
- Session replay

**Not responsible for:**
- Executing tools
- File mutation
- Policy enforcement
- Prompt construction

### Control Plane

The authoritative component. Responsibilities:

1. **Session Lifecycle** - Create, manage, terminate sessions
2. **Context Assembly** - Build prompts with proper token budgeting
3. **Policy Enforcement** - Apply trust tiers and rules
4. **Approval Workflow** - Gate sensitive operations
5. **Audit Logging** - Record all events

### Agent Runtime

Handles the AI interaction loop:

1. **Planning** - Decide what tools to use
2. **Tool Selection** - Choose appropriate tools
3. **Model Invocation** - Call AI with context
4. **Observation/Action** - Process results, decide next step
5. **Retry Logic** - Handle failures gracefully
6. **Stop Conditions** - Know when done

### Tool Layer

Provides structured tools:

- **Read tools** - File reading, code search
- **Write tools** - Patch application, file creation
- **Execute tools** - Shell commands, tests, linting

Each tool:
- Has a defined input schema
- Returns structured output
- Is tracked in audit log

## Data Flow

### Turn Execution Flow

```
User: "Fix the bug in auth.go"
   │
   ▼
TUI sends request to Control Plane
   │
   ▼
Control Plane:
  1. Load session state
  2. Assemble context (files, history)
  3. Apply token budget
  4. Build prompt
   │
   ▼
Agent Runtime:
  1. Analyze request
  2. Select tools
  3. Call model
  4. Process response
   │
   ▼
If tool requires approval:
  1. Emit approval event
  2. Wait for user decision
  3. Execute or reject
   │
   ▼
Tool executes in sandbox
   │
   ▼
Results returned to agent
   │
   ▼
Agent continues or completes
   │
   ▼
Events logged to audit store
   │
   ▼
TUI displays results
```

## State Management

### Session State

```
active → completed
      → terminated
```

### Turn State

```
active → awaiting_approval → active → completed
       → completed
       → failed
       → cancelled
```

### Approval State

```
proposed → pending → approved → executed
                     → denied  → cancelled
```

## Security Model

### Trust Tiers

Trust tiers control:
- Resource limits (timeout, output size)
- Tool permissions
- Approval requirements

Higher tiers = more trust = fewer restrictions

### Sandbox

Tools execute in a constrained environment:
- Allowed commands only
- Working directory restricted to repo
- Timeout limits
- Output size limits

### Audit

Every action is logged:
- Who did what
- When it happened
- What the result was

This enables:
- Session replay
- Security auditing
- Compliance tracking

## Persistence

### SQLite Database

All data stored locally in SQLite:
- Sessions
- Turns
- Events
- Artifacts

Uses WAL mode for concurrent access.

### Event Store

Immutable event log:
- Append-only
- Structured payloads
- Queryable by type
- Filterable by session

## Extensibility

### Providers

Support for multiple AI providers:
- Anthropic
- OpenAI

Selectable via policy.

### MCP Integration

Model Context Protocol support for:
- Plugin tools
- External capabilities

### Custom Tools

Tools defined with:
- Name and description
- Input schema (JSON Schema)
- Execution handler
