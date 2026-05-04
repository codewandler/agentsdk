# Requirements refinement checklist

Use this checklist when helping a user turn a vague app idea into concrete requirements.

## Essential questions

1. **Users**: Who will use this agent? (developers, ops, end-users, other agents)
2. **Jobs-to-be-done**: What specific tasks should the agent perform?
3. **Input/output**: What does the agent receive and what does it produce?
4. **Tools needed**: What external tools, CLIs, APIs, or services must the agent access?
5. **Workflows**: Are there repeatable multi-step processes to automate?
6. **Commands**: What slash commands should users be able to invoke?
7. **Skills**: What domain knowledge should the agent have? Are there existing skills to reuse?
8. **Data boundaries**: What data can the agent access? What is off-limits?
9. **Safety**: What destructive operations need confirmation? What should be read-only?
10. **Acceptance criteria**: How will we know the agent works correctly?

## Exploration steps

Before finalizing requirements, explore the integration surface:

- Run `which <tool>` and `<tool> --help` for any CLI the agent will use.
- Check if existing agentsdk skills cover the domain (`/skills` or `ls ~/.agents/skills/`).
- Look at example apps in the agentsdk `examples/` directory for similar patterns.
- Search the web for API documentation if the agent needs external service access.

## Output format

Summarize refined requirements as:

```markdown
## App: <name>
### Users
### Jobs
### Tools & integrations
### Workflows
### Commands
### Skills
### Safety constraints
### Acceptance criteria
```

Confirm assumptions with the user before scaffolding.
