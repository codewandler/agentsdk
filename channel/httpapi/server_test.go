package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/harness"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/stretchr/testify/require"
)

func TestNativeHealthOpenListAndCommand(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodGet, server.URL+nativePrefix+"/health", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"health":"ok"`)

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions", map[string]any{"name": "web", "agent_name": "coder"})
	require.Equal(t, http.StatusCreated, resp.Code)
	require.Contains(t, resp.Body.String(), `"Name":"web"`)

	resp = requestJSON(t, http.MethodGet, server.URL+nativePrefix+"/sessions", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"Name":"web"`)

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions/web/commands", map[string]any{"path": []string{"session", "info"}})
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"kind":"display"`)
	require.Contains(t, resp.Body.String(), `"display"`)
	require.Contains(t, resp.Body.String(), `session:`)
}

func TestNativeContextEndpointExposesDescriptorsAndSnapshot(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions", map[string]any{"name": "web", "agent_name": "coder"})
	require.Equal(t, http.StatusCreated, resp.Code)

	resp = requestJSON(t, http.MethodGet, server.URL+nativePrefix+"/sessions/web/context", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"agent":"coder"`)
	require.Contains(t, resp.Body.String(), `"key":"environment"`)
	require.Contains(t, resp.Body.String(), `"descriptors"`)

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions/web/commands", map[string]any{"path": []string{"turn"}, "input": map[string]any{"text": "hello"}})
	require.Equal(t, http.StatusOK, resp.Code)

	resp = requestJSON(t, http.MethodGet, server.URL+nativePrefix+"/sessions/web/context", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"providers"`)
	require.Contains(t, resp.Body.String(), `"fragments"`)
}

func TestNativeWorkflowStartAndRunsRequireThreadBackedSession(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions", map[string]any{"name": "web", "agent_name": "coder"})
	require.Equal(t, http.StatusCreated, resp.Code)

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions/web/workflows/echo/start", map[string]any{"input": "hello"})
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"Data":"hello"`)

	resp = requestJSON(t, http.MethodGet, server.URL+nativePrefix+"/sessions/web/workflows/runs", nil)
	require.Equal(t, http.StatusBadRequest, resp.Code)
	require.Contains(t, resp.Body.String(), "thread-backed")
}

func TestNativeEventStreamReceivesCommandEvent(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions", map[string]any{"name": "web", "agent_name": "coder"})
	require.Equal(t, http.StatusCreated, resp.Code)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+nativePrefix+"/sessions/web/events", nil)
	require.NoError(t, err)
	eventsResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer eventsResp.Body.Close()
	require.Equal(t, http.StatusOK, eventsResp.StatusCode)

	done := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(eventsResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				done <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
	}()

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions/web/commands", map[string]any{"path": []string{"workflow", "list"}})
	require.Equal(t, http.StatusOK, resp.Code)

	select {
	case data := <-done:
		require.Contains(t, data, `"type":"command"`)
		require.Contains(t, data, `"command_path":["workflow","list"]`)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE command event")
	}
}

func TestAGUICompatibilityNamespaceAdvertisesBoundary(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodGet, server.URL+"/ag-ui/v1", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"protocol":"ag-ui"`)
	require.Contains(t, resp.Body.String(), "protocol-neutral")
	require.Contains(t, resp.Body.String(), "A2UI")
}
func TestSlashStringsAreNotNativeCommandAPI(t *testing.T) {
	service := testService(t)
	server := httptest.NewServer(New(service).Handler())
	defer server.Close()

	resp := requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions", map[string]any{"name": "web", "agent_name": "coder"})
	require.Equal(t, http.StatusCreated, resp.Code)

	resp = requestJSON(t, http.MethodPost, server.URL+nativePrefix+"/sessions/web/commands", map[string]any{"command": "/workflow list"})
	require.Equal(t, http.StatusBadRequest, resp.Code)
	require.Contains(t, resp.Body.String(), "unknown field")
}

func testService(t *testing.T) *harness.Service {
	t.Helper()
	a, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", System: "You help."}),
		app.WithDefaultAgent("coder"),
		app.WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
			return action.OK(input)
		})),
		app.WithWorkflows(workflow.Definition{Name: "echo", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}}),
		app.WithAgentOptions(agent.WithClient(runnertest.NewClient(runnertest.TextStream("ok")))),
	)
	require.NoError(t, err)
	return harness.NewService(a)
}

type testHTTPResponse struct {
	Code int
	Body bytes.Buffer
}

func requestJSON(t *testing.T, method string, url string, body any) testHTTPResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out testHTTPResponse
	out.Code = resp.StatusCode
	_, err = io.Copy(&out.Body, resp.Body)
	require.NoError(t, err)
	return out
}
