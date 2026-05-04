---
name: testing
description: Target app discovery and smoke testing guidance
---
# Testing target apps

The builder is the tester/controller. The current project is the system under test.

Use `builder_discover_target` to inspect target descriptors and `builder_run_target_smoke` for non-destructive checks. Target app test sessions are isolated under `.agentsdk/builder/target-sessions`.

Do not assume target tools are safe. Prefer discovery, command listing, workflow listing, and explicit user-approved prompts.
