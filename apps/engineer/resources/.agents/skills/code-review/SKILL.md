---
name: code-review
description: Review code changes for correctness, clarity, and maintainability.
---
# Code Review Skill

Use this skill when the user asks for a code review, feedback on a diff, or
guidance on improving existing code.

Before reviewing, gather context:

- Use `git_diff` or `file_read` to see the actual changes.
- Read surrounding code to understand existing patterns and conventions.
- Check for related tests — are they updated?

Review checklist (in priority order):

1. **Correctness** — Does the code do what it claims? Check edge cases, error
   handling, nil/zero-value paths, off-by-one conditions, and concurrency
   safety. Verify error messages are actionable.
2. **Security** — Flag injection risks, missing input validation, hardcoded
   secrets, overly broad permissions, and unsafe deserialization.
3. **Clarity** — Can a new team member understand the intent without extra
   context? Flag unclear names, implicit assumptions, magic values, and
   comments that restate the code instead of explaining why.
4. **Maintainability** — Is the change easy to modify later? Watch for tight
   coupling, duplicated logic, missing abstractions, and changes that make
   future work harder.
5. **Testing** — Are the important behaviors covered? Identify untested paths
   and suggest the smallest useful tests. Prefer table-driven tests for
   multiple cases.
6. **Performance** — Flag only concrete concerns: unnecessary allocations in
   hot paths, O(n²) where O(n) is straightforward, missing indexes, unbounded
   growth. Do not speculate about performance without evidence.
7. **Style** — Follow the project's existing conventions. Do not impose
   external style preferences. Only flag style issues that hurt readability.

Output format:

- One-sentence summary of what the change does
- Blocking issues (must fix before merge)
- Suggestions (would improve the change)
- Nits (take or leave)
- Untested paths and suggested test cases
- Overall assessment: approve, request changes, or needs discussion
