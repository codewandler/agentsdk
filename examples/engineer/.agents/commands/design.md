---
description: Produce a lightweight architecture design for a feature or system change.
argument-hint: "<feature or system change to design>"
---
Create a lightweight architecture design for:

{{.Query}}

Read the existing codebase first to understand current patterns, boundaries, and
conventions. Use `grep`, `dir_tree`, and `file_read` to explore.

Include:

- problem statement and constraints
- proposed component boundaries and responsibilities
- data flow between components
- key interfaces or contracts
- trade-offs considered and the rationale for the chosen approach
- what is explicitly out of scope
- risks and open questions
- a verification plan to confirm the design works
