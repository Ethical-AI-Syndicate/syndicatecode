# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.0] - 2026-03-12

### Added

- **Multi-Provider AI Support**
  - Anthropic streaming provider with full tool support
  - OpenAI streaming provider with tool message handling
  - Provider registry for dynamic provider selection
  - Provider-agnostic LanguageModel interface

- **Audit System**
  - Event store with SQLite backend (WAL mode)
  - FileMutationRecord for tracking file changes
  - RecordToolInvocation for tracking tool executions
  - RecordModelInvocation for tracking model calls
  - Event type constants with test coverage

- **Context Management**
  - BudgetAllocator wired into ContextAssembler
  - Token budget allocation across system prompt, repo context, and conversation history

- **Tool System**
  - Tool registry with validation
  - Tool execution with file mutation recording
  - Execution result handling

- **TUI Features**
  - Command aliases for improved UX
  - Approval lifecycle validation
  - Replay mode with read-only enforcement
  - Command-to-endpoint mapping contract tests

### Changed

- **Control Plane**
  - Close remaining architecture gaps
  - Enforce runtime safeguards
  - Reconcile export handler format
  - Remove unused types

- **OpenAI Provider**
  - Fix StopReason accumulation
  - Handle system prompt and tool blocks
  - Emit MessageStartEvent with input tokens
  - Fix streaming ID correlation

- **CI/CD**
  - Use Go 1.25 to match go.mod requirement
  - Handle Beads unavailability gracefully
  - Exempt merge commits from bead verification

### Fixed

- **Security**
  - Suppress gosec false positives
  - Fail closed on approval execution faults
  - Bead evidence lookup hardened for missing beads
  - Skip closure checks for unresolved beads

- **Control Plane**
  - Concurrent mutable turns rejection with read-only access preservation

- **TUI**
  - Merge duplicate start/new-session and ask/turn cases

### Security

- Bead-exempt SHA allowlist for merge commits

[Unreleased]: https://gitlab.mikeholownych.com/ai-syndicate/syndicatecode/compare/v0.9.0...HEAD
[0.9.0]: https://gitlab.mikeholownych.com/ai-syndicate/syndicatecode/releases/v0.9.0
