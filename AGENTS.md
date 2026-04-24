# AGENTS.md - agentsdk notes

This file is for developers and AI agents working on agentsdk and its nearby
consumer repos.

## Dependency update process

When upgrading `llmadapter`, pass the released version through the dependency
chain deliberately:

1. Verify or cut the `llmadapter` release.
2. Update `agentsdk` to that released `llmadapter` version.
3. Run `go test ./...` in `agentsdk`.
4. Commit, tag, and push the `agentsdk` release.
5. Update consumers such as `../miniagent` to the released `agentsdk` version
   and the same direct `llmadapter` version when they import it directly.
6. Run consumer tests.
7. For CLI consumers, reinstall the compiled binary before smoke testing.

Important: `miniagent` is a compiled Go binary. Updating `go.mod`, tagging, or
pushing repos does not update the already installed `$GOPATH/bin/miniagent`.
After dependency-chain updates, run `task install` in `../miniagent` before
checking installed-binary behavior.

If `llmadapter resolve <model>` works but `miniagent -m <model>` fails, first
verify that the installed `miniagent` binary was rebuilt after the dependency
update. Otherwise the binary may still contain older routing behavior.
