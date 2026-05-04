---
name: deployment
description: Deployment guidance for generated agentsdk apps
---
# Deployment guidance

Deployment is guidance-only in builder v1.

Help users identify:

- runtime command (`agentsdk run`, `agentsdk serve`, or a hybrid app binary);
- session storage path;
- environment variables and secrets;
- CI checks (`go test ./...`, `agentsdk discover --local .`);
- rollback and observability basics.

Generate deployment assets only after explicit confirmation.
