package jsonquery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

type testCtx struct {
	action.BaseCtx
	workDir string
}

func (c testCtx) WorkDir() string       { return c.workDir }
func (c testCtx) AgentID() string       { return "test-agent" }
func (c testCtx) SessionID() string     { return "test-session" }
func (c testCtx) Extra() map[string]any { return nil }

func ctx(dir string) tool.Ctx { return testCtx{BaseCtx: action.BaseCtx{Context: context.Background()}, workDir: dir} }

func TestJSONQuery_FieldWildcard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"items":[{"name":"alpha"},{"name":"beta"}]}`), 0644))

	raw, _ := json.Marshal(QueryParams{Path: "data.json", Expr: ".items[].name"})
	res, err := Tool().Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	require.Contains(t, res.String(), "Matches: 2")
	require.Contains(t, res.String(), `"alpha"`)
	require.Contains(t, res.String(), `"beta"`)
}

func TestJSONQuery_IndexAndQuotedField(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"items":[{"display name":"alpha"},{"display name":"beta"}]}`), 0644))

	raw, _ := json.Marshal(QueryParams{Path: "data.json", Expr: `.items[1]["display name"]`})
	res, err := Tool().Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	require.Contains(t, res.String(), `"beta"`)
	require.NotContains(t, res.String(), `"alpha"`)
}

func TestJSONQuery_LimitsResults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"items":[1,2,3]}`), 0644))

	raw, _ := json.Marshal(QueryParams{Path: "data.json", Expr: ".items[]", MaxResults: 2})
	res, err := Tool().Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	require.Contains(t, res.String(), "Matches: 3 (showing 2)")
	require.Contains(t, res.String(), "[0] 1")
	require.Contains(t, res.String(), "[1] 2")
	require.NotContains(t, res.String(), "[2] 3")
}

func TestJSONQuery_ParseErrorsAreToolErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"items":`), 0644))

	raw, _ := json.Marshal(QueryParams{Path: "data.json", Expr: ".items"})
	res, err := Tool().Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "parse JSON")
}

func TestJSONQuery_InvalidExpression(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{}`), 0644))

	raw, _ := json.Marshal(QueryParams{Path: "data.json", Expr: "items"})
	res, err := Tool().Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "expression must start")
}

func TestJSONQuery_Tools(t *testing.T) {
	tools := Tools()
	require.Len(t, tools, 1)
	require.Equal(t, "json_query", tools[0].Name())
	require.True(t, strings.Contains(tools[0].Description(), "JSON"))
}

func TestJSONQuery_ActionExecutes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"items":[{"name":"alpha"}]}`), 0644))

	result := Action().Execute(ctx(dir), QueryParams{Path: "data.json", Expr: ".items[].name"})
	require.NoError(t, result.Error)
	res, ok := result.Data.(tool.Result)
	require.True(t, ok)
	require.False(t, res.IsError(), res.String())
	require.Contains(t, res.String(), `"alpha"`)
}

func TestJSONQuery_ActionDeclaresIntent(t *testing.T) {
	dir := t.TempDir()
	intent := action.ExtractIntent(Action(), ctx(dir), QueryParams{Path: "data.json", Expr: ".items"})
	require.Equal(t, "json_query", intent.Action)
	require.Equal(t, "filesystem_read", intent.Class)
	require.Equal(t, "high", intent.Confidence)
	require.Len(t, intent.Operations, 1)
	require.Equal(t, "workspace", intent.Operations[0].Resource.Locality)
}
