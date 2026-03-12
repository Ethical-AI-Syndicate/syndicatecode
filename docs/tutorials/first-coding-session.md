# Your First Coding Session

This tutorial walks you through making your first code changes with SyndicateCode, including reviewing and approving edits.

## What you'll learn

- Inspect code using the AI agent
- Review proposed changes (patches)
- Approve or deny tool executions
- Track what happened in your session

## Prerequisites

- Completed [Getting Started](getting-started.md)
- A Git repository with some code to modify

## Step 1: Start a session

In your TUI, create a new session:

```
start /path/to/your/repo tier1
```

Note the session ID returned (you can also list sessions with `sessions`).

## Step 2: Ask the agent to make a change

Send a request:

```
ask Add a log statement to the main function
```

## Step 3: Observe the agent's actions

The agent will:
1. Read files to understand the codebase
2. Search for the main function
3. Propose a patch

Watch the event stream to see each step. You'll see:
- `tool_use_start` - A tool is being called
- `tool_input_delta` - The tool's input is being built
- Text deltas showing the model's reasoning

## Step 4: Review approval requests

When the agent wants to run a tool that requires approval, you'll see:

```
awaiting_approval
```

View pending approvals:

```
approvals
```

This shows each approval request with:
- The tool being proposed
- The reason for the request
- Any context about the action

## Step 5: Approve or deny

To approve a tool execution:

```
approve <approval_id>
```

To deny:

```
deny <approval_id>
```

You can get the approval ID from the `approvals` command.

## Step 6: Review the result

After approval, the tool executes and returns results. The agent continues its work, possibly proposing more changes.

To see what changed in your repository, use your regular Git commands:

```bash
git diff
```

## Step 7: Review session history

After your session, replay what happened:

```
replay <session_id>
```

This shows all events in order:
- Tool invocations
- Approval decisions
- Model responses
- File changes

## Understanding the workflow

Here's what happens during a typical session:

```
┌─────────┐     ┌──────────────┐     ┌─────────┐
│  You    │────▶│ Control Plane│◀────│  AI    │
└─────────┘     └──────────────┘     └─────────┘
      │                │                    │
      │   ask <msg>    │                    │
      │───────────────▶│                    │
      │                │  Analyze           │
      │                │───────────────────▶│
      │                │                    │
      │                │◀───────────────────│
      │                │                    │
      │    Propose     │                    │
      │◀───────────────│                    │
      │                │                    │
      │  approve 123   │                    │
      │───────────────▶│                    │
      │                │  Execute tool      │
      │                │───────────────────▶│
      │                │◀───────────────────│
      │                │                    │
      │    Results     │                    │
      │◀───────────────│                    │
```

## Tips for effective sessions

### Be specific in your requests

Instead of:
```
ask Fix the code
```

Try:
```
ask Add error handling to the parseConfig function in config.go
```

### Review each approval

Take time to understand what each tool will do. The approval shows:
- The exact file paths affected
- The operation being performed
- The reason for the action

### Use tier0 for exploration

When exploring unfamiliar code, start with `tier0`. This limits what the agent can do without approval, keeping you in control.

## Next steps

- [Configure Trust Tiers](../how-to/configure-trust-tiers.md) - Set up custom limits
- [Review Session History](../how-to/review-session-history.md) - Full replay capabilities
- [Export Session Data](../how-to/export-session-data.md) - Save sessions for audit
