Act as a senior system architect and perform a production-level review of this project, including both the AI specification (ai-spec) and the codebase. Your review should cover:
1. Production Readiness
    * Evaluate whether the codebase meets production-quality standards (code quality, structure, scalability, reliability, security, observability).
2. AI-Spec Consistency
    * Verify that the implementation aligns with the ai-spec.
    * Identify mismatches, deviations, or incomplete features.
3. Spec Gaps
    * Identify anything implemented in the codebase but missing or unclear in the ai-spec.
    * Suggest improvements to make the ai-spec fully reflect the system.
4. Phase Completeness
    * Highlight missing components, edge cases, or unfinished work.
    * Scope: Full codebase + AI-spec (BE + FE), Phase 1, 2, 3, 4, 5 completeness, connected example app
5. Actionable Recommendations
    * Provide clear, prioritized recommendations to reach production-grade quality.
    * For every problem identified (bug, gap, redundancy, inconsistency, or incomplete feature) provide a concrete, actionable solution.
    * Output is Markdown (.md) file, Concise, Completion percentages
    * Write jobs files (.yaml) for OpenCode to fix gaps