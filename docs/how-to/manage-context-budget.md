# Manage Context Budget

This guide explains how token budgets work and how to manage them effectively.

## What is context budget

Context budget controls how many tokens are available for each part of the AI interaction:

- **System prompt** - Instructions that define agent behavior
- **Repository context** - Code files, structure, documentation
- **Conversation history** - Previous turns in the session

Managing this budget ensures:
- Consistent performance
- Predictable costs
- Optimal use of the model's context window

## How budgets are allocated

The system uses a BudgetAllocator to distribute tokens:

```
Total budget (e.g., 100,000 tokens)
├── System prompt: 4,000 tokens (reserved)
├── Repository context: ~80,000 tokens
│   ├── Recent files: 40,000
│   ├── Important files: 20,000
│   └── Structure: 20,000
└── Conversation history: ~16,000 tokens
```

## Viewing current budget

To see budget allocation for a turn:

```
context <session_id> <turn_id>
```

This shows:
- Total tokens available
- Tokens used per category
- Remaining budget

## Factors affecting budget

### Trust tier

Higher trust tiers typically allow larger budgets:
- tier0: 32K tokens
- tier1: 64K tokens
- tier2: 128K tokens
- tier3: 200K tokens

### Repository size

Larger repositories may have:
- More files to include
- Less history (to stay within budget)
- Prioritized file selection

### Turn complexity

Complex turns may require:
- More conversation history
- Additional context
- Higher token usage

## Optimizing budget usage

### Be specific in requests

Instead of:
```
ask Fix the code
```

Use:
```
ask Add null check to parseConfig function in config.go line 42
```

This helps the agent find relevant code without loading unnecessary files.

### Use file paths

When possible, include specific file paths:
```
ask Add logging to /internal/server.go
```

This targets context to specific files.

### Break complex tasks

Large tasks use more tokens. Break into smaller sessions:
```
# Instead of one large task
ask Refactor the entire auth system

# Use multiple focused tasks
ask Extract auth logic to separate package
ask Add tests for auth package
```

## Context selection strategy

The system ranks and selects context based on:

1. **Relevance** - Files most related to the task
2. **Recency** - Recently modified files
3. **Importance** - Core vs. generated files
4. **Size** - Prefer smaller, focused files

## Monitoring usage

Track token usage across sessions:

```
metrics
```

This shows:
- Tokens per request
- Average context size
- Budget utilization

## Budget and performance

Larger budgets mean:
- More context available to the AI
- Better-informed responses
- Higher API costs

Balance between:
- Context quality (more tokens)
- Performance (fewer tokens)
- Cost control (token limits)

## Best practices

### Start focused

Begin with specific, bounded requests. Expand scope only as needed.

### Monitor metrics

Watch token usage trends to understand your patterns.

### Adjust tier if needed

If consistently hitting limits:
- Upgrade to higher tier for more budget
- Or break tasks into smaller pieces

### Clear session for new tasks

For unrelated tasks, start a new session. This provides fresh context rather than mixing topics.
