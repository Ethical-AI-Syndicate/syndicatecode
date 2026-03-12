# Getting Started

This tutorial guides you through setting up SyndicateCode and running your first session.

## What you'll learn

- Install and configure SyndicateCode
- Start the control plane server
- Connect the TUI client
- Verify your setup is working

## Prerequisites

- Go 1.25 or later
- A Git repository you want to work with

## Step 1: Build the application

Clone the repository and build:

```bash
git clone https://gitlab.mikeholownych.com/ai-syndicate/syndicatecode.git
cd syndicatecode
go build ./...
```

This compiles both the server and TUI client.

## Step 2: Start the control plane

The control plane is the core service that manages sessions, enforces policies, and coordinates tool execution.

In one terminal, start the server:

```bash
go run ./cmd/server
```

You should see output like:

```
Starting control plane server on :7777
```

The server listens on port 7777 by default. You can change this with the `ADDR` environment variable or configuration.

## Step 3: Connect the TUI client

Open a second terminal and start the TUI:

```bash
go run ./cmd/cli
```

Or connect to a remote control plane:

```bash
SYNDICATE_CONTROLPLANE_URL=http://192.168.1.100:7777 go run ./cmd/cli
```

You should see:

```
SyndicateCode TUI
Type 'help' for commands
>
```

## Step 4: Verify with health check

In the TUI, type:

```
health
```

You should see a response indicating the server is healthy, including version information.

## Step 5: Create your first session

Start a new coding session:

```
start /path/to/your/repo tier1
```

Replace `/path/to/your/repo` with an actual path to a Git repository. The `tier1` argument sets the trust tier (tier0, tier1, tier2, tier3).

You should receive a response confirming the session was created with a session ID.

## Step 6: Send a message

Now you can interact with the AI:

```
ask Fix the bug in the login function
```

The agent will analyze your code and may:
- Read files to understand the codebase
- Search for specific code patterns
- Propose changes that require your approval

## Understanding trust tiers

When starting a session, you choose a trust tier:

| Tier | Description | Output Limit | Timeout |
|------|-------------|--------------|---------|
| tier0 | Most restrictive | 64 KB | 30s |
| tier1 | Standard | 256 KB | 120s |
| tier2 | Elevated | 512 KB | 300s |
| tier3 | Full access | 1 MB | 600s |

Start with `tier1` for most tasks.

## Next steps

- [Your First Coding Session](first-coding-session.md) - Make your first changes with approval
- [Configure Trust Tiers](../how-to/configure-trust-tiers.md) - Customize limits
- [Use Approval Workflows](../how-to/use-approval-workflows.md) - Review and approve tool calls
