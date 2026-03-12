# Use Approval Workflows

This guide explains how to review and approve tool executions during a session.

## Why approvals exist

Approvals ensure you review potentially sensitive operations before they execute. This includes:
- File modifications
- Shell command execution
- External service calls
- Any operation that changes state

## Viewing pending approvals

During a session, the TUI notifies you when approval is needed:

```
awaiting_approval
```

To see all pending approvals:

```
approvals
```

Each approval shows:
- **Approval ID** - Unique identifier for this request
- **Tool name** - What tool wants to run
- **Session ID** - Which session this belongs to
- **Proposed state** - What the tool will do

## Approving an action

To approve:

```
approve <approval_id>
```

Example:
```
approve abc123
```

The tool then executes and returns results to the agent.

## Denying an action

To deny:

```
deny <approval_id>
```

Example:
```
deny abc123
```

Provide a reason if appropriate - this is recorded in the audit log.

## Understanding approval states

Each approval goes through a lifecycle:

```
proposed → pending → approved/denied → executed/cancelled
```

- **proposed**: Agent wants to run the tool
- **pending**: Awaiting your decision
- **approved**: You said yes, tool executes
- **denied**: You said no, tool does not execute
- **executed**: Tool completed successfully
- **cancelled**: Tool was cancelled (by agent or system)

## Approval examples

### Example 1: Approving a file edit

```
> ask Add error handling to auth.go
...
awaiting_approval
> approvals
ID: approval-xyz
Tool: apply_patch
File: auth.go
Operation: Add try-catch block at line 42
> approve approval-xyz
```

### Example 2: Denying a shell command

```
> ask Run the tests
...
awaiting_approval
> approvals
ID: approval-abc
Tool: restricted_shell
Command: go test ./...
WorkingDir: /project
> deny approval-abc
Reason: Running tests is safe, but let's verify the output first
```

## Best practices

### Review the context

Before approving, consider:
- Is this operation safe?
- Does it match what you asked for?
- Are the file paths correct?
- Is the command appropriate?

### Don't approve blindly

Take time to understand what each approval means. The approval request includes:
- Exact tool being called
- Parameters being passed
- Target files or resources

### Start with denials if unsure

If you're uncertain, deny the request. The agent can:
- Explain the operation in more detail
- Propose an alternative approach
- Break down the task into smaller steps

## Viewing approval history

To see past approval decisions:

```
replay <session_id>
```

Filter for approval events:
- `approval_proposed`
- `approval_approved`
- `approval_denied`
- `approval_executed`

This creates a complete audit trail of all decisions.
