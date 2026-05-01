---
name: devops
description: Advise on CI/CD pipelines, deployment strategies, and infrastructure automation.
---
# DevOps Skill

Use this skill for CI/CD pipeline design, deployment planning, infrastructure
automation, and production readiness review.

When advising on DevOps topics:

1. **Pipelines** — Keep build steps reproducible and cacheable. Separate build,
   test, lint, and deploy stages. Fail fast on the cheapest checks first.
   Pin tool versions. Use matrix builds for cross-platform targets.
2. **Deployments** — Prefer rolling or blue-green deployments. Define rollback
   triggers before deploying, not after. Include health checks and smoke tests
   as gate conditions. Specify the exact commands and config changes needed.
3. **Infrastructure** — Treat infrastructure as code. Pin provider and module
   versions, use declarative configuration, and keep secrets out of
   repositories. Prefer managed services over self-hosted when the team is
   small.
4. **Observability** — Every deployed service needs health endpoints, structured
   logs, and key metrics (latency p50/p95/p99, error rate, saturation). Alert
   on symptoms (error rate spike, latency increase), not causes (CPU high).
   Include runbook links in alert definitions.
5. **Security** — Apply least-privilege to service accounts and IAM roles. Scan
   dependencies for known vulnerabilities in CI. Rotate credentials on a
   schedule. Use short-lived tokens over long-lived secrets.
6. **Reliability** — Identify single points of failure. Design for graceful
   degradation: circuit breakers, timeouts, retry budgets. Document recovery
   procedures and test them periodically.
7. **Cost** — Flag resource over-provisioning. Recommend right-sizing,
   autoscaling policies, and spot/preemptible instances where appropriate.

Tie every recommendation to a concrete action: a config file to create, a
command to run, or a setting to change. Avoid generic best-practice lists
without project context.

When creating deployment checklists, always include:

- Pre-deployment gates (tests, approvals, config review)
- Ordered deployment steps with exact commands
- Post-deployment verification (health checks, smoke tests, metric baselines)
- Rollback criteria and procedure
- Communication checkpoints
