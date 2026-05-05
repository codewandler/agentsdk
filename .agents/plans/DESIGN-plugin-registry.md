# DESIGN: Global Plugin Registry

Status: Idea  
Date: 2026-05-05

## Problem

The `localcli.Factory` is a manual switch statement that maps plugin names to
constructors. Every new plugin requires editing this file. This doesn't scale
and creates an unnecessary coupling between the host factory and all plugin
packages.

## Idea

A global plugin registry (similar to `database/sql` driver registration or
`image` format registration in Go stdlib) where plugins self-register via
`init()` or an explicit `Register()` call.

```go
// plugins/registry.go
package plugins

var registry = map[string]FactoryFunc{}

type FactoryFunc func(config map[string]any) (app.Plugin, error)

func Register(name string, factory FactoryFunc) {
    registry[name] = factory
}

func Get(name string, config map[string]any) (app.Plugin, error) {
    f, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("plugin %q not registered", name)
    }
    return f(config)
}
```

Each plugin registers itself:
```go
// plugins/browserplugin/register.go
func init() {
    plugins.Register("browser", func(config map[string]any) (app.Plugin, error) {
        return newFromConfig(config), nil
    })
}
```

## Trade-offs

| Pro | Con |
|-----|-----|
| No manual switch statement | `init()` magic — harder to trace |
| Adding a plugin = one file | Import side-effects required |
| Decoupled from host factory | Need a "batteries" import for default set |

## Decision

Defer. Current manual factory works fine with <10 plugins. Revisit when the
plugin count makes the switch statement painful or when external/third-party
plugins become a requirement.
