# Hybrid app example

This example combines declarative resources with host/plugin configuration. The manifest loads `.agents` resources and names plugin refs with structured config; Go code or an embedding host can still add actions/plugins at runtime.

Try:

```bash
agentsdk discover --local examples/hybrid-app
agentsdk run examples/hybrid-app /workflow list
```
