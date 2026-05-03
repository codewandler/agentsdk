package action

import "errors"

// Status classifies the outcome of an action execution without prescribing a
// projection-specific response shape.
type Status string

const (
	// StatusOK means the action completed successfully.
	StatusOK Status = "ok"
	// StatusError means the action failed and Result.Error should describe why.
	StatusError Status = "error"
)

// OK returns a successful Result with data.
func OK(data any, events ...Event) Result {
	return Result{Status: StatusOK, Data: data, Events: events}
}

// Failed returns an error Result. A nil error still produces an error-status
// result so middleware and adapters can preserve explicit failure contracts.
func Failed(err error, events ...Event) Result {
	return Result{Status: StatusError, Error: err, Events: events}
}

// IsError reports whether the result represents a failed action.
func (r Result) IsError() bool {
	return r.Status == StatusError || r.Error != nil
}

// Err returns the result error. If the result is marked failed without a
// concrete error, Err returns a generic sentinel instead of nil.
func (r Result) Err() error {
	if r.Error != nil {
		return r.Error
	}
	if r.Status == StatusError {
		return errors.New("action failed")
	}
	return nil
}
