Act as a senior system architect and product-focused engineer.

Your task is to perform a deep functional analysis of the provided project (codebase, specs, APIs, workflows, and related artifacts).

## 1. Functional Flow Understanding
- Reconstruct ALL end-to-end functional flows of the system.
    - Identify core user journeys and business processes.
    - Map interactions across services, modules, APIs, and data stores.
    - Clearly describe:
        - Entry points (UI, API, events, schedulers)
        - Processing steps (validation, transformation, business logic)
        - State transitions
        - Outputs (responses, DB writes, events, side effects)
- Highlight implicit flows not clearly documented but inferred from code.

## 2. Functional Correctness Validation
- Verify whether each flow satisfies intended business logic.
- Detect:
    - Missing steps or incomplete flows
    - Incorrect business rules or logic violations
    - Broken state transitions
    - Data inconsistencies across services
    - Race conditions affecting functional correctness
    - Edge cases not handled (nulls, retries, partial failures)

## 3. Issue Detection
For every issue found:
- Clearly describe:
    - What is wrong
    - Where it occurs (file/module/service)
    - A reproducible scenario (if possible)
- Classify severity:
    - Critical (data loss, incorrect business outcome)
    - High (user-visible incorrect behavior)
    - Medium (edge-case failure)
    - Low (minor inconsistency)

## 4. Root Cause Analysis
- Go beyond symptoms and identify the REAL root cause:
    - Design flaw (wrong architecture, missing boundaries)
    - Logic bug (incorrect condition, branching)
    - State management issue
    - Concurrency or ordering issue
    - Data contract mismatch between services
- Trace cause → effect across the system

## 5. Recommendations (Actionable Fixes)
For EACH issue:
- Provide a concrete fix, not generic advice:
    - Code-level fix (pseudo-code or pattern)
    - Architectural improvement (if needed)
    - Data model correction
    - API contract adjustment
- Suggest best practices or patterns:
    - Idempotency
    - Saga / transaction boundaries
    - Validation layers
    - Domain-driven design improvements
- Highlight trade-offs of the proposed fix

## 6. Flow-Level Risk Assessment
- Identify flows that are:
    - Fragile under change
    - Hard to reason about
    - Prone to future bugs
- Suggest simplification or redesign where necessary

## 7. Output Format
Structure your response as:

1. System Functional Flow Overview
2. Reconstructed End-to-End Flows
3. Detected Functional Issues (Table)
4. Root Cause Analysis (Detailed)
5. Recommended Fixes (Concrete + Prioritized)
6. High-Risk Areas & Design Weaknesses

## Important Rules
- Be precise and technical — avoid vague statements.
- Do NOT assume correctness — verify everything.
- Focus on real behavior from code, not just intention.
- Prioritize depth over breadth.
- Think like an engineer responsible for production failures.

Your goal is to expose hidden functional problems and provide fixes that can be directly implemented.