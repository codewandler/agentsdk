package contextproviders

import "github.com/codewandler/agentsdk/agentcontext"

func (p *EnvironmentProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "process working directory, OS, architecture, kernel, and hostname", Lifecycle: "agent-baseline", Scope: agentcontext.CacheThread, CachePolicy: agentcontext.CachePolicy{Stable: true, Scope: agentcontext.CacheThread}}
}

func (p *TimeProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "current time bucket", Lifecycle: "agent-baseline", Scope: agentcontext.CacheTurn, CachePolicy: agentcontext.CachePolicy{MaxAge: p.bucketInterval(), Scope: agentcontext.CacheTurn}}
}

func (p *CmdProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "command-derived key/value context", Lifecycle: "configured-provider", Scope: p.cache.Scope, CachePolicy: p.cache}
}

func (p *FileProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "file-backed context fragments", Lifecycle: "configured-provider", Scope: p.cache.Scope, CachePolicy: p.cache}
}

func (p *GitProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "git repository state", Lifecycle: "agent-baseline", Scope: agentcontext.CacheTurn, CachePolicy: agentcontext.CachePolicy{Scope: agentcontext.CacheTurn}}
}

func (p *ProjectInventoryProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "project file inventory summary", Lifecycle: "configured-provider", Scope: agentcontext.CacheTurn, CachePolicy: agentcontext.CachePolicy{Scope: agentcontext.CacheTurn}}
}

func (p staticProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "static context fragment", Lifecycle: "agent-baseline", Scope: p.fragment.CachePolicy.Scope, CachePolicy: p.fragment.CachePolicy}
}

func (p staticSetProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "static context fragment set", Lifecycle: "agent-baseline", Scope: agentcontext.CacheThread, CachePolicy: agentcontext.CachePolicy{Stable: true, Scope: agentcontext.CacheThread}}
}

func (p skillInventoryProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{Key: p.Key(), Description: "skill catalog and activation state", Lifecycle: "agent-local", Scope: agentcontext.CacheThread, CachePolicy: agentcontext.CachePolicy{Stable: true, Scope: agentcontext.CacheThread}}
}
