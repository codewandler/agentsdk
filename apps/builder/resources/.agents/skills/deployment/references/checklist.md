# Deployment checklist

Deployment is guidance-only in builder v1. Generate deployment assets only after
explicit user confirmation.

## Runtime options

| Command | Use case |
|---------|----------|
| `agentsdk run <dir>` | Interactive terminal session |
| `agentsdk serve <dir>` | Daemon mode with triggers and channels |
| Custom Go binary | Hybrid app with `app.New(...)` and custom wiring |

## Pre-deployment checklist

1. **Discovery clean**: `agentsdk discover --local .` reports no diagnostics.
2. **Smoke tests pass**: `builder_run_target_smoke` all checks passed.
3. **README complete**: Documents purpose, setup, usage, and environment variables.
4. **Environment variables**: All required secrets and config documented.
5. **Session storage**: Session directory path configured and writable.
6. **Model access**: LLM provider credentials available in the target environment.

## CI integration

```bash
# Minimal CI check for a resource-only app
agentsdk discover --local .
# For hybrid apps, also run Go tests
go test ./...
```

## Environment variables to document

- `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` — LLM provider credentials
- `TAVILY_API_KEY` — web search (if `web_search` tool is used)
- App-specific variables for external integrations

## Rollback

Resource-only apps are stateless code — rollback is a git revert.
Session data lives in the sessions directory and is independent of app code.
