# GitNexus Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate GitNexus knowledge graph into Open-Guard for AI agent impact analysis

**Architecture:** Local-first workflow - GitNexus analysis runs locally, index stored in `.gitnexus/` directory, agent queries it via `make gitnexus-analyze` or pre-commit hook. Full codebase indexed.

**Tech Stack:** GitNexus CLI, Makefile, git hooks

---

## Task 1: Install GitNexus CLI

**Files:**
- Modify: `Makefile` - Add GitNexus installation and commands

- [ ] **Step 1: Research GitNexus installation method**

Run: `websearch GitNexus CLI installation method 2026`
Get the installation command (npm, brew, or direct download)

- [ ] **Step 2: Add GitNexus installation to Makefile**

```makefile
## GitNexus
GITNEXUS_VERSION := latest

install-gitnexus:
	@echo "Installing GitNexus..."
# Add platform-specific install command
gitnexus --version
```

Save to: `Makefile`

- [ ] **Step 3: Test installation**

Run: `make install-gitnexus`
Expected: GitNexus installed, version displayed

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "feat: add GitNexus installation to Makefile"
```

---

## Task 2: Configure GitNexus Index

**Files:**
- Create: `.gitnexus/config.yaml` - Index configuration

- [ ] **Step 1: Create .gitnexus directory**

```bash
mkdir -p .gitnexus
```

- [ ] **Step 2: Create configuration file**

```yaml
# .gitnexus/config.yaml
index:
  # Full codebase - Go services + TypeScript + shared packages
  paths:
    - services/
    - packages/
    - web/
    - apps/
    - examples/
  
  # Language settings
  languages:
    - go
    - typescript
    - javascript
  
  # Exclude test files from call chains (optional, configurable)
  include_tests: true

detectors:
  # Custom patterns for Open-Guard architecture
  - name: context-usage
    pattern: "context\\.(Context|TODO)"
    description: "Detect context.Context usage"
  
  - name: rls-violation
    pattern: "db\\.(Exec|Query).*WithoutOrgID"
    description: "Detect potential RLS violations"
  
  - name: kafka-topics
    pattern: "kafka\\.(Produce|Consume).*topic:"
    description: "Detect Kafka topic relationships"
```

- [ ] **Step 3: Verify config is valid**

Run: `gitnexus config validate .gitnexus/config.yaml`
Expected: Valid configuration

- [ ] **Step 4: Commit**

```bash
git add .gitnexus/
git commit -m "feat: add GitNexus configuration"
```

---

## Task 3: Create GitNexus Analysis Command

**Files:**
- Modify: `Makefile` - Add analyze command

- [ ] **Step 1: Add analyze command to Makefile**

```makefile
## GitNexus Analysis
.PHONY: gitnexus-analyze

gitnexus-analyze: gitnexus-install
	@echo "Running GitNexus analysis..."
	@mkdir -p .gitnexus
	gitnexus analyze --config .gitnexus/config.yaml --output .gitnexus/index.json
	@echo "Analysis complete. Index saved to .gitnexus/index.json"
```

- [ ] **Step 2: Test the command**

Run: `make gitnexus-analyze`
Expected: Analysis runs, generates index

- [ ] **Step 3: Add .gitignore entry**

Add to `.gitignore`:
```
.gitnexus/
```

- [ ] **Step 4: Commit**

```bash
git add Makefile .gitignore
git commit -m "feat: add GitNexus analyze command"
```

---

## Task 4: Set Up Git Hooks

**Files:**
- Create: `.git/hooks/post-commit` - Post-commit hook
- Create: `.git/hooks/pre-push` - Pre-push hook (optional)

- [ ] **Step 1: Create post-commit hook**

```bash
#!/bin/bash
# .git/hooks/post-commit
# Auto-analyze after commits (optional, can be slow)

# Check if GitNexus is installed
if command -v gitnexus &> /dev/null; then
    echo "Running post-commit GitNexus analysis..."
    gitnexus analyze --config .gitnexus/config.yaml --output .gitnexus/index.json 2>/dev/null || true
fi
```

- [ ] **Step 2: Make hooks executable**

```bash
chmod +x .git/hooks/post-commit
```

- [ ] **Step 3: Test hook exists**

Run: `ls -la .git/hooks/post-commit`
Expected: File exists and is executable

- [ ] **Step 4: Commit**

```bash
git add .git/hooks/
git commit -m "feat: add GitNexus post-commit hook"
```

**Note:** Git hooks in `.git/hooks/` are not tracked by git. Consider using a tool like `husky` or `lefthook` if hooks should be committed. If so, skip Task 4 and use proper hook management instead.

---

## Task 5: Create Custom Detectors

**Files:**
- Modify: `.gitnexus/config.yaml` - Add custom detectors

- [ ] **Step 1: Add context.Context detector**

```yaml
detectors:
  - name: context-usage
    pattern: '\\bcontext\\.(Context|TODO)\\b'
    description: "Detect context.Context usage for RLS enforcement"
    severity: warning
```

- [ ] **Step 2: Add RLS violation detector**

```yaml
  - name: rls-call-chain
    pattern: '(db|pgx|sqlx)\.(Exec|Query).*'
    description: "Track database calls for RLS validation"
    context: requires-org-id
```

- [ ] **Step 3: Add Kafka topic detector**

```yaml
  - name: kafka-producer
    pattern: 'kafka\\.(Produce|ProduceAsync)\\('
    description: "Track Kafka producers"
  
  - name: kafka-consumer
    pattern: 'kafka\\.(Consume|Subscribe)\\('
    description: "Track Kafka consumers"
```

- [ ] **Step 4: Test detectors**

Run: `gitnexus analyze --config .gitnexus/config.yaml --detectors`
Expected: Custom detectors loaded

- [ ] **Step 5: Commit**

```bash
git add .gitnexus/config.yaml
git commit -m "feat: add custom Open-Guard detectors for GitNexus"
```

---

## Task 6: Integrate with ai-check

**Files:**
- Modify: `Makefile` - Integrate GitNexus into ai-check

- [ ] **Step 1: Add gitnexus query to ai-check**

Find current `ai-check` target in Makefile and add:

```makefile
ai-check: gitnexus-analyze golangci-lint sqlfluff
	@echo "Running GitNexus impact check..."
	@if [ -f .gitnexus/index.json ]; then \
		gitnexus impact --since HEAD~1 --file .gitnexus/index.json; \
	else \
		echo "No GitNexus index found. Run 'make gitnexus-analyze' first."; \
	fi
```

- [ ] **Step 2: Test ai-check integration**

Run: `make ai-check`
Expected: ai-check runs with GitNexus query

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: integrate GitNexus with ai-check target"
```

---

## Task 7: Documentation

**Files:**
- Create: `docs/gitnexus-guide.md` - Usage guide

- [ ] **Step 1: Create usage guide**

```markdown
# GitNexus Guide

## Quick Start

1. Install GitNexus:
   ```bash
   make install-gitnexus
   ```

2. Run initial analysis:
   ```bash
   make gitnexus-analyze
   ```

3. Query impact:
   ```bash
   gitnexus impact --file .gitnexus/index.json --symbol packages/core/src/utils.ts
   ```

## Workflows

### Local Analysis (default)
- Runs on-demand: `make gitnexus-analyze`
- Auto-runs after commit (if post-commit hook configured)

### Using with ai-check
- `make ai-check` includes GitNexus impact analysis
- Shows call chains for changed files

### Custom Detectors
- `context.Usage` - Finds all context.Context usage
- `rls-call-chain` - Tracks database calls
- `kafka-producer/consumer` - Tracks Kafka topics

## Troubleshooting

### Index not found
Run: `make gitnexus-analyze`

### Analysis slow
Reduce paths in `.gitnexus/config.yaml` to only critical directories

### Hook not running
Check: `chmod +x .git/hooks/post-commit`
```

- [ ] **Step 2: Commit**

```bash
git add docs/gitnexus-guide.md
git commit -m "docs: add GitNexus usage guide"
```

---

## Execution Summary

| Task | Description | Dependencies |
|------|------------|-------------|
| 1 | Install GitNexus CLI | None |
| 2 | Configure GitNexus index | Task 1 |
| 3 | Create analyze command | Task 2 |
| 4 | Set up git hooks | Task 3 |
| 5 | Create custom detectors | Task 2 |
| 6 | Integrate with ai-check | Task 3, Task 5 |
| 7 | Add documentation | Task 6 |

**Total: 7 tasks**

**Next step:** Choose execution approach:
1. **Subagent-Driven (recommended)** - Fresh subagent per task with two-stage review
2. **Inline Execution** - Execute tasks in session with checkpoints