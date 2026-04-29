package tool

import "context"

// Approver is called when a tool call needs human approval.
// It is defined in the tool package so it can be carried via context without
// creating import cycles between tool and toolmw.
//
// Parameters:
//   - ctx: the tool execution context
//   - intent: what the tool is about to do (resources, operations, behaviors).
//     Until the intent system is implemented (Phase 2), this will be a zero-value Intent.
//   - detail: optional assessment detail from the risk gate. Consumers that need
//     rich information (e.g. TUI renderers) can type-assert to the concrete
//     assessment type from toolmw. Simple approvers (CI deny-all, allowlist)
//     can ignore it.
//
// Returns true if approved, false if denied.
type Approver func(ctx Ctx, intent Intent, detail any) (bool, error)

type approverKey struct{}

// ApproverFrom extracts the Approver from a context.
// Returns nil if no approver is set.
func ApproverFrom(ctx context.Context) Approver {
	if v := ctx.Value(approverKey{}); v != nil {
		if a, ok := v.(Approver); ok {
			return a
		}
	}
	return nil
}

// CtxWithApprover returns a new context with the given Approver.
// This is typically called once at the app/runtime layer to inject the
// approval mechanism (TUI prompt, CI deny-all, allowlist, etc.).
func CtxWithApprover(ctx context.Context, approver Approver) context.Context {
	return context.WithValue(ctx, approverKey{}, approver)
}
