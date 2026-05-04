---
name: app-architecture
description: App architecture guidance for agentsdk apps
---
# App architecture

Design agentsdk apps using boring boundaries:

- resources for agents, skills, commands, workflows, triggers, and declarative metadata;
- actions for typed Go-native execution;
- tools for LLM-facing capabilities;
- workflows for repeatable orchestration;
- harness/session APIs for live execution and testing;
- daemon/channel packages for hosting and presentation.

Start resource-only unless the app truly needs Go-native helper actions or plugins.
