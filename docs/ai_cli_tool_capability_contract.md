# AI Coding CLI – Tool Capability Contract

Tools expose capabilities to the agent runtime. Each tool must declare a contract that defines its inputs, outputs, side effects, and policy constraints.

---

# Tool Definition Structure

Example tool manifest:

```yaml
name: read_file
version: 1
side_effect: none
approval_required: false
input_schema:
  path: string
output_schema:
  content: string
  size: integer
```

---

# Capability Fields

## name

Unique identifier of the tool.

---

## version

Version number for compatibility.

---

## side_effect

Possible values:

- none
- read
- write
- execute
- network

---

## approval_required

Defines whether user approval is needed before execution.

---

# Input Schema

Tools must define a structured schema for arguments.

Example:

```yaml
input_schema:
  path:
    type: string
    description: file path
```

Structured arguments prevent prompt injection from producing arbitrary commands.

---

# Output Schema

Outputs must also be structured.

Example:

```yaml
output_schema:
  diff:
    type: string
  files_changed:
    type: array
```

---

# Execution Constraints

Tools must define limits:

- max execution time
- max output size
- working directory
- allowed paths

Example:

```yaml
limits:
  timeout_seconds: 60
  max_output_bytes: 500000
```

---

# Security Metadata

Tools should declare:

```yaml
security:
  network_access: false
  filesystem_scope: repo_only
```

---

# Tool Lifecycle

Execution flow:

1. Agent proposes tool invocation
2. Control plane validates schema
3. Policy evaluated
4. Approval requested if required
5. Tool executed
6. Result recorded as event

---

# Example Tool – Apply Patch

```yaml
name: apply_patch
version: 1
side_effect: write
approval_required: true
input_schema:
  patch: string
output_schema:
  files_modified: array
  validation:
    syntax: string
    format: string
```

---

# Tool Governance

Tools should be registered centrally.

Capabilities must be visible to operators.

Plugins introducing tools must declare identical manifests.

