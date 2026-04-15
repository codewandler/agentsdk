package turn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// ── TurnDoneResult.String() ───────────────────────────────────────────────────

func TestTurnDoneResult_String(t *testing.T) {
	tests := []struct {
		name    string
		result  TurnDoneResult
		wantOut string
	}{
		{
			name:    "errMsg and content both non-empty",
			result:  TurnDoneResult{errMsg: "something went wrong", content: "extra detail"},
			wantOut: "Turn failed: something went wrong\nextra detail",
		},
		{
			name:    "errMsg non-empty content empty",
			result:  TurnDoneResult{errMsg: "bad request", content: ""},
			wantOut: "Turn failed: bad request",
		},
		{
			name:    "errMsg empty content non-empty",
			result:  TurnDoneResult{errMsg: "", content: "all good"},
			wantOut: "all good",
		},
		{
			name:    "both empty",
			result:  TurnDoneResult{errMsg: "", content: ""},
			wantOut: "Turn done.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantOut, tc.result.String())
		})
	}
}

// ── TurnDoneResult.IsError() ─────────────────────────────────────────────────

func TestTurnDoneResult_IsError(t *testing.T) {
	tests := []struct {
		name      string
		result    TurnDoneResult
		wantError bool
	}{
		{
			name:      "errMsg non-empty returns true",
			result:    TurnDoneResult{errMsg: "oops"},
			wantError: true,
		},
		{
			name:      "errMsg empty returns false",
			result:    TurnDoneResult{errMsg: ""},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantError, tc.result.IsError())
		})
	}
}

// ── TurnDoneResult.IsTurnDone() ──────────────────────────────────────────────

func TestTurnDoneResult_IsTurnDone(t *testing.T) {
	require.True(t, TurnDoneResult{}.IsTurnDone())
	require.True(t, TurnDoneResult{errMsg: "e", content: "c"}.IsTurnDone())
}

// ── Tool().Execute() ─────────────────────────────────────────────────────────

func TestTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		params        TurnDoneParams
		wantNilRes    bool
		wantErr       bool
		wantToolError bool // result is non-nil but IsError() == true (tool.Error path)
		wantContent   string
		wantErrMsg    string
	}{
		{
			// Empty params are an expected user-side mistake; the tool returns
			// tool.Error (a non-nil Result) so the LLM sees the message and can retry.
			name:          "both Content and Error empty returns tool.Error result no Go error",
			params:        TurnDoneParams{Content: "", Error: ""},
			wantNilRes:    false,
			wantErr:       false,
			wantToolError: true,
		},
		{
			name:        "Content provided returns TurnDoneResult with content no error",
			params:      TurnDoneParams{Content: "hello world", Error: ""},
			wantNilRes:  false,
			wantErr:     false,
			wantContent: "hello world",
			wantErrMsg:  "",
		},
		{
			name:        "Error provided returns TurnDoneResult with errMsg no error",
			params:      TurnDoneParams{Content: "", Error: "something failed"},
			wantNilRes:  false,
			wantErr:     false,
			wantContent: "",
			wantErrMsg:  "something failed",
		},
		{
			name:        "Both Content and Error provided returns TurnDoneResult with both no error",
			params:      TurnDoneParams{Content: "partial output", Error: "also failed"},
			wantNilRes:  false,
			wantErr:     false,
			wantContent: "partial output",
			wantErrMsg:  "also failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tl := Tool()
			raw, err := json.Marshal(tc.params)
			require.NoError(t, err)

			res, execErr := tl.Execute(nil, raw)

			if tc.wantErr {
				require.Error(t, execErr)
				require.Nil(t, res)
				return
			}

			require.NoError(t, execErr)
			require.NotNil(t, res)

			if tc.wantToolError {
				require.True(t, res.IsError(), "expected tool error result")
				return
			}

			got, ok := res.(TurnDoneResult)
			require.True(t, ok, "expected TurnDoneResult, got %T", res)
			require.Equal(t, tc.wantContent, got.content)
			require.Equal(t, tc.wantErrMsg, got.errMsg)
		})
	}
}
