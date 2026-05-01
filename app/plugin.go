// Package app composes agents, commands, plugins, and frontends.
package app

import (
	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/datasource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/workflow"
)

// Plugin is a named contribution bundle. Plugins may implement any of the
// optional contribution interfaces below.
type Plugin interface {
	Name() string
}

type CommandsPlugin interface {
	Plugin
	Commands() []command.Command
}

type AgentSpecsPlugin interface {
	Plugin
	AgentSpecs() []agent.Spec
}

type ToolsPlugin interface {
	Plugin
	Tools() []tool.Tool
}

type ActionsPlugin interface {
	Plugin
	Actions() []action.Action
}

type DataSourcesPlugin interface {
	Plugin
	DataSources() []datasource.Definition
}

type WorkflowsPlugin interface {
	Plugin
	Workflows() []workflow.Definition
}
type SkillsPlugin interface {
	Plugin
	SkillSources() []skill.Source
}

// ContextProvidersPlugin contributes app-scoped context providers that are
// registered on every agent instantiated through the App. These providers
// must be stateless or config-only — they are created once at plugin
// registration time, not per agent instance.
type ContextProvidersPlugin interface {
	Plugin
	ContextProviders() []agentcontext.Provider
}

// AgentContextPlugin contributes context providers that depend on per-agent
// runtime state. The factory is called during agent instantiation (after skill
// and tool initialization), not at plugin registration time.
type AgentContextPlugin interface {
	Plugin
	AgentContextProviders(AgentContextInfo) []agentcontext.Provider
}

// ToolMiddlewarePlugin contributes middlewares applied to all tools
// after all tools have been registered (two-pass: tools first, then middlewares).
type ToolMiddlewarePlugin interface {
	Plugin
	ToolMiddlewares() []tool.Middleware
}

// ToolTargetedMiddlewarePlugin contributes middlewares for specific tools.
// ToolMiddlewaresFor is called once per registered tool name (exact, not glob).
// Return nil to skip a tool. Applied before global middlewares (innermost).
type ToolTargetedMiddlewarePlugin interface {
	Plugin
	ToolMiddlewaresFor(toolName string) []tool.Middleware
}

// AgentContextInfo carries the per-agent state available when
// [AgentContextPlugin.AgentContextProviders] is called.
//
// This struct mirrors [agent.ContextProviderFactoryInfo]. When adding fields
// here, update ContextProviderFactoryInfo and the bridge in
// [App.InstantiateAgent].
type AgentContextInfo struct {
	SkillRepository *skill.Repository
	SkillState      *skill.ActivationState
	ActiveTools     func() []tool.Tool // closure over the agent's toolset
	Workspace       string
	Model           string
	Effort          string
}
