# Release Notes

## v0.9.0 - Initial Release

**Released**: March 12, 2026

Welcome to SyndicateCode! This is the first stable release of our AI coding CLI with a local control plane for secure, auditable agentic coding assistance.

## What's New

### Multi-Provider AI Support

You can now use multiple AI providers with SyndicateCode:

- **Anthropic Models**: Claude integration with full streaming support
- **OpenAI Models**: GPT integration with tool message handling

The provider registry allows dynamic provider selection based on your needs.

### Full Audit Trail

Every operation is now tracked:

- **Tool Invocations**: See exactly what tools were called, when, and their results
- **File Mutations**: Track all file changes made during a session
- **Model Calls**: Monitor AI model usage and routing decisions
- **Event Replay**: Replay entire sessions for debugging and auditing

### Improved Context Management

Token budgeting ensures efficient use of AI context windows:

- Separate budgets for system prompts, repository context, and conversation history
- Smart allocation based on task requirements

### Approval Workflows

Sensitive operations require approval:

- Tool executions can be gated behind approval requirements
- Full lifecycle tracking: proposed → pending → approved/denied → executed

## Improvements

- **Security**: Fail-closed behavior on approval execution faults
- **TUI**: Command aliases for faster navigation
- **Reliability**: Better error handling and validation

## Architecture

SyndicateCode follows a layered architecture:

```
TUI Client → Control Plane → Agent Runtime → Tool Runners → Local Repo
```

The control plane is the authoritative component—UI clients remain thin.

## Getting Started

```bash
# Start the server
go run ./cmd/server

# In another terminal, run the CLI
go run ./cmd/cli
```

## Full Changelog

See [CHANGELOG.md](CHANGELOG.md) for complete details.

---

**Documentation**: See the `docs/` folder for architecture specs and API documentation.
