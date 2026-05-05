package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/harness"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestE2ELocalCLIPluginHarnessLoadAndSessionCommandProjection(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	loaded, err := Load(t.Context(), Config{
		Resources:    ResolvedResources(agentdir.Resolution{}),
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &bytes.Buffer{},
		Err:          &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.NotNil(t, loaded.Harness)
	require.Equal(t, "default", loaded.AgentName)

	require.NoError(t, loaded.Agent.RunTurn(t.Context(), 1, "hello"))
	require.Contains(t, e2eRequestToolNames(client.RequestAt(0).Tools), harness.AgentCommandToolName)
	require.Contains(t, loaded.Agent.ContextState(), string(harness.AgentCommandCatalogProviderKey))

	sessionTool := loaded.Session.AgentCommandProjection().Tools[0]
	res, err := sessionTool.Execute(e2eToolCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"path":["workflow","list"]}`))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "No workflows registered")

	res, err = sessionTool.Execute(e2eToolCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"path":["workflow","start"],"input":{"name":"missing"}}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "not callable")
}

func TestE2EOneShotCommandRenderingAndNoDefaultPlugins(t *testing.T) {
	client := runnertest.NewClient()
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		Task:         "/session info",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Empty(t, client.Requests())
	require.Contains(t, out.String(), "session:")
	require.Contains(t, out.String(), "agent: coder")

	err = Run(t.Context(), Config{
		Resources:        ResolvedResources(agentdir.Resolution{}),
		Task:             "hello",
		Workspace:        t.TempDir(),
		NoDefaultPlugins: true,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:              &bytes.Buffer{},
		Err:              &bytes.Buffer{},
	})
	require.ErrorContains(t, err, "no agents found")
}

func TestE2EWorkflowStartRunsAndResumeSessionLookup(t *testing.T) {
	workspace := t.TempDir()
	sessionsDir := filepath.Join(workspace, "sessions")
	resources := EmbeddedResources(e2eExplicitWorkflowBundle(), ".agents")
	appOptions := []app.Option{app.WithActions(action.New(action.Spec{Name: "echo.e2e"}, func(_ action.Ctx, input any) action.Result {
		return action.Result{Data: input}
	}))}
	var startOut bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:        resources,
		Task:             "/workflow start echo_flow hello from e2e",
		Workspace:        workspace,
		SessionsDir:      sessionsDir,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		AppOptions:       appOptions,
		NoDefaultPlugins: true,
		Out:              &startOut,
		Err:              &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Contains(t, startOut.String(), "workflow completed: echo_flow")
	require.Contains(t, startOut.String(), "output: hello from e2e")
	runID := e2eWorkflowRunID(startOut.String())
	require.NotEmpty(t, runID)

	var runsOut bytes.Buffer
	err = Run(t.Context(), Config{
		Resources:        resources,
		Task:             "/workflow runs",
		Workspace:        workspace,
		SessionsDir:      sessionsDir,
		ContinueLast:     true,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		AppOptions:       appOptions,
		NoDefaultPlugins: true,
		Out:              &runsOut,
		Err:              &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Contains(t, runsOut.String(), runID)
	require.Contains(t, runsOut.String(), "echo_flow")

	var runOut bytes.Buffer
	err = Run(t.Context(), Config{
		Resources:        resources,
		Task:             "/workflow run " + runID,
		Workspace:        workspace,
		SessionsDir:      sessionsDir,
		ContinueLast:     true,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		AppOptions:       appOptions,
		NoDefaultPlugins: true,
		Out:              &runOut,
		Err:              &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Contains(t, runOut.String(), "workflow run: "+runID)
	require.Contains(t, runOut.String(), "output: hello from e2e")
}

func TestE2EAsyncWorkflowCompletesInLongLivedCLIRepl(t *testing.T) {
	workspace := t.TempDir()
	sessionsDir := filepath.Join(workspace, "sessions")
	resources := EmbeddedResources(e2eSlowWorkflowBundle(), ".agents")
	appOptions := []app.Option{app.WithActions(action.New(action.Spec{Name: "slow.e2e"}, func(ctx action.Ctx, input any) action.Result {
		select {
		case <-ctx.Done():
			return action.Result{Error: ctx.Err()}
		case <-time.After(100 * time.Millisecond):
			return action.Result{Data: input}
		}
	}))}
	in, writer := io.Pipe()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		_, _ = io.WriteString(writer, "/workflow start slow_flow hello --async\n")
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(writer, "/workflow runs\n")
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(writer, "/workflow runs\n")
		_, _ = io.WriteString(writer, "/quit\n")
	}()

	err := Run(t.Context(), Config{
		Resources:        resources,
		Workspace:        workspace,
		SessionsDir:      sessionsDir,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		AppOptions:       appOptions,
		NoDefaultPlugins: true,
		In:               in,
		Out:              &out,
		Err:              &bytes.Buffer{},
	})
	<-done
	require.NoError(t, err)
	text := out.String()
	require.Contains(t, text, "workflow started: slow_flow")
	require.Contains(t, text, "status: queued")
	require.Contains(t, text, "slow_flow")
	require.Contains(t, text, "succeeded")
}

func TestE2EAppManifestPluginRefsLoad(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "agentsdk.app.json"), []byte(`{"sources":[".agents"],"plugins":["manifest_plugin"]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, ".agents", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, ".agents", "agents", "coder.md"), []byte("---\nname: coder\n---\nsystem"), 0o644))
	factory := &e2ePluginFactory{}

	loaded, err := Load(t.Context(), Config{
		Resources:        DirResources(workspace),
		Workspace:        workspace,
		NoDefaultPlugins: true,
		PluginFactory:    factory,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:              &bytes.Buffer{},
		Err:              &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, []string{"manifest_plugin"}, factory.names)
}

func e2eExplicitWorkflowBundle() fstest.MapFS {
	bundle := testBundle()
	bundle[".agents/workflows/echo.yaml"] = &fstest.MapFile{Data: []byte(`name: echo_flow
description: Echo through an explicit app action
steps:
  - id: echo
    action: echo.e2e
`)}
	return bundle
}

func e2eSlowWorkflowBundle() fstest.MapFS {
	bundle := testBundle()
	bundle[".agents/workflows/slow.yaml"] = &fstest.MapFile{Data: []byte(`name: slow_flow
description: Slow async workflow
steps:
  - id: slow
    action: slow.e2e
`)}
	return bundle
}

func e2eWorkflowRunID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if id, ok := strings.CutPrefix(line, "run: "); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func e2eRequestToolNames(tools []unified.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

type e2eToolCtx struct {
	action.BaseCtx
}

func (c e2eToolCtx) WorkDir() string       { return "" }
func (c e2eToolCtx) AgentID() string       { return "coder" }
func (c e2eToolCtx) SessionID() string     { return "session" }
func (c e2eToolCtx) Extra() map[string]any { return nil }

type e2ePluginFactory struct {
	names []string
}

func (f *e2ePluginFactory) PluginForName(_ context.Context, name string, _ map[string]any) (app.Plugin, error) {
	f.names = append(f.names, name)
	return e2eNamedPlugin(name), nil
}

type e2eNamedPlugin string

func (p e2eNamedPlugin) Name() string { return string(p) }
