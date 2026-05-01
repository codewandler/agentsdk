---
description: Create a deployment checklist for a service or change.
argument-hint: "<service, release, or change to deploy>"
---
Create a deployment checklist for:

{{.Query}}

Read the project's existing CI/CD configuration, Dockerfiles, Makefiles, or
deployment scripts first. Use `grep`, `dir_tree`, and `file_read` to find them.

Include:

- pre-deployment checks (tests green, dependencies pinned, config reviewed)
- deployment steps in order with exact commands
- health checks and smoke tests to run after deployment
- rollback criteria and procedure
- monitoring to watch during and after rollout (specific metrics and thresholds)
- communication checkpoints (team, stakeholders, on-call)
- post-deployment cleanup or follow-up tasks
