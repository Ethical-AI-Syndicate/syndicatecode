# AI Coding CLI – Threat Model

## Assets

Primary assets that must be protected:

- repository source code
- secrets in files or environment
- local filesystem outside repository
- shell execution capability
- git credentials
- session logs
- prompt data sent to model providers
- approval records
- policy configuration

---

# Threat Actors

## 1. Curious Operator

Non‑malicious user who may approve unsafe actions or misunderstand diffs.

---

## 2. Malicious Repository Content

Prompt injection embedded in:

- README files
- code comments
- failing test output
- documentation

---

## 3. Malicious Plugin or MCP Server

Plugin that exfiltrates data or manipulates tool outputs.

---

## 4. Model Provider Risk

Sensitive prompt data may be retained or processed externally.

---

## 5. Compromised Tool or Dependency

Supply‑chain attack within local tools.

---

## 6. Host Environment Manipulation

Malicious PATH entries, environment variables, or shell aliases.

---

# Threat Scenarios

## Scenario 1 – Prompt Injection

### Attack

Repository text instructs the model to ignore rules and expose secrets.

### Consequence

Sensitive information may be revealed or unsafe actions performed.

### Controls

- treat repo text as untrusted
- instruction hierarchy
- secret‑exclusion rules
- approval barriers

---

## Scenario 2 – Secret Exfiltration

### Attack

Agent reads sensitive files and includes them in prompts.

### Consequence

Credentials leak to model provider or logs.

### Controls

- path‑based restrictions
- secret scanning
- prompt redaction
- provider routing

---

## Scenario 3 – Shell Abuse

### Attack

Agent runs destructive or exfiltration commands.

### Consequence

Data loss or credential exposure.

### Controls

- restricted shell tool
- command allowlists
- network restrictions
- approval requirements

---

## Scenario 4 – Malicious Plugin

### Attack

Plugin gains access to repository context and sends it externally.

### Consequence

Confidential data leakage.

### Controls

- plugin capability manifests
- plugin trust tiers
- event logging
- restricted network access

---

## Scenario 5 – Context Truncation

### Attack

Large context causes important instructions to be dropped.

### Consequence

Model operates on incomplete state.

### Controls

- deterministic token budgeting
- truncation visibility
- priority rules for critical context

---

## Scenario 6 – Audit Log Leakage

### Attack

Sensitive information appears in session logs.

### Consequence

Persistent local leakage.

### Controls

- redacted storage
- retention controls
- export filtering

---

## Scenario 7 – Approval Theater

### Attack

User approves vague action summary.

### Consequence

Action differs from what the user intended.

### Controls

- approvals bound to exact arguments
- path display
- side‑effect classification

---

# Security Principles

## Untrusted Text Principle

Repository content is treated as **untrusted evidence**, not authoritative instruction.

---

## Policy Enforcement Principle

Policy must be enforced outside the model runtime.

---

## Capability Separation

Separate permissions for:

- read
- write
- execute
- network

---

## Causality Preservation

Every outcome must be traceable to:

- context
- decision
- action

---

## Bounded Autonomy

Agent actions must remain interruptible and inspectable.

