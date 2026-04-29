# AI Coding Rules (Open-Guard)

These rules are strict boundaries for AI agents operating in this repository to minimize token cost and avoid hallucination.

1. **Never Load the Full Repository**: Do not use "read all files" or greedy glob patterns to dump the entire repo into context.
2. **Max 3 Files Per Task**: Try to locate the 1-3 files you need for a task and ONLY read/edit those. 
3. **Use AI Entry Points**: Start by reading the `README_AI.md` in the relevant module/service folder to find the exact file to edit.
4. **Use Tags for Navigation**: Use the `make index` command locally to generate or search a `tags` file if you need to find exact function definitions across the repo.
5. **No Long Explanations**: When confirming tasks, be brief. 
