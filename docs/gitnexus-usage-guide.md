# GitNexus Usage Guide

GitNexus provides AI-powered code intelligence for Open-Guard. It maps symbols, relationships, and execution flows into a queryable knowledge graph.

> **Index Status:** 7,394 symbols, 11,896 relationships, 265 execution flows.

## Quick Start

### 1. Install

```bash
npm install -D gitnexus
```

### 2. Index the Codebase

```bash
npx gitnexus analyze
```

This builds the knowledge graph and generates context files. Run after major code changes or when tools report stale index.

### 3. First Analysis

```bash
npx gitnexus status
```

Verify the index loaded correctly. Look for symbol/relationship counts.

## Querying Impact

### Finding What Uses a Symbol

```bash
# Using the MCP tool
gitnexus_impact({target: "validateUser", direction: "upstream"})
```

Output:

| Depth | Risk | Callers |
| ----- | ---- | ------- |
| d=1 | **WILL BREAK** | loginHandler, apiMiddleware |
| d=2 | LIKELY AFFECTED | authRouter, sessionManager |

### Blast Radius Levels

| Level | Meaning |
| ----- | ------- |
| d=1 (direct) | These WILL BREAK if you change the symbol |
| d=2 (indirect) | LIKELY AFFECTED through intermediate dependencies |
| d=3 (transitive) | MAY NEED TESTING for edge cases |

### Pre-Commit Safety Check

```bash
gitnexus_impact({target: "validateUser", direction: "upstream"})
gitnexus_detect_changes()
```

Verify changes only affect expected symbols before committing.

## Exploring Code

### Understanding a Concept

```bash
gitnexus_query({query: "authentication flow"})
```

Returns execution flows (processes) and related symbols ranked by relevance.

### Getting Symbol Context

```bash
gitnexus_context({name: "processPayment"})
```

Output:

```
Incoming calls: checkoutHandler, webhookHandler
Outgoing calls: validateCard, chargeStripe, saveTransaction
Processes: CheckoutFlow (step 3/7), RefundFlow (step 1/4)
```

### Tracing Execution Flows

Read `gitnexus://repo/Open-Guard/process/{name}` for step-by-step traces:

```
gitnexus://repo/Open-Guard/process/LoginFlow
→ Step 1: receiveCredentials
→ Step 2: validateUser
→ Step 3: checkRateLimit
→ Step 4: issueToken
→ Step 5: logAuthEvent
```

## Custom Detectors

GitNexus supports custom code pattern detection via YAML configuration.

### Creating a Custom Detector

Place detector files in `.gitnexus/detectors/`:

```yaml
# .gitnexus/detectors/open-guard-patterns.yaml
patterns:
  - id: "openGuard.missing-context"
    description: "Functions missing context.Context parameter"
    severity: "warning"
    ast:
      type: "FunctionDeclaration"
      without:
        params:
          type: "Ident"
          names: ["ctx", "context"]

  - id: "openGuard.rls-violation"
    description: "Database queries without RLS session"
    severity: "error"
    ast:
      type: "CallExpression"
      callee:
        type: "MemberExpression"
        property: ["Query", "Exec", "QueryRow"]
      without:
        ancestor:
          type: "FunctionDeclaration"
          body:
            contains:
              call: "db.WithOrgID"
```

### Running Detectors

```bash
npx gitnexus detect --detectors .gitnexus/detectors/open-guard-patterns.yaml
```

### AGENTS.md Integration

Open-Guard includes pre-built detectors in `.opencode/`:

- `phase5-*.yaml`: Phase 5 detector patterns
- `phase6-*.yaml`: Compliance check patterns
- `phase7-*.yaml`: Security scan patterns
- `phase10-*.yaml`: DLP detector patterns

## Troubleshooting

### "Index is stale"

```bash
npx gitnexus analyze --force
```

### "Not inside a git repository"

Ensure you are in a directory inside a git repo. GitNexus requires git metadata.

### Tools not responding

Restart Claude Code to reload the MCP server after re-indexing.

### Slow indexing

Omit `--embeddings` flag (default). Embeddings are optional and add overhead.

### Wrong results

Run `npx gitnexus status` to verify index freshness. Large PRs may require re-index.

## Skills Reference

| Task | Skill File |
| ---- | --------- |
| Understand architecture | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius analysis | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Debugging trace | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Safe refactoring | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |
| Full reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |

## Resources

| Resource | URL |
| -------- | --- |
| Codebase context | `gitnexus://repo/Open-Guard/context` |
| Functional areas | `gitnexus://repo/Open-Guard/clusters` |
| All execution flows | `gitnexus://repo/Open-Guard/processes` |
| Graph schema | `gitnexus://repo/Open-Guard/schema` |