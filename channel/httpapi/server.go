package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/harness"
)

const nativePrefix = "/api/agentsdk/v1"

// Server exposes a small HTTP/SSE channel over harness.Service. It is an adapter:
// command execution, session lifecycle, and workflow semantics remain owned by
// harness.Session and harness.Service.
type Server struct {
	Service *harness.Service
}

func New(service *harness.Service) *Server { return &Server{Service: service} }

func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	if path == "/ag-ui/v1" {
		s.handleAGUIInfo(w, r)
		return
	}
	if !strings.HasPrefix(path, nativePrefix) {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(path, nativePrefix)
	if rel == "" {
		rel = "/"
	}
	switch {
	case r.Method == http.MethodGet && rel == "/health":
		s.handleHealth(w, r)
	case r.Method == http.MethodGet && rel == "/sessions":
		s.handleListSessions(w, r)
	case r.Method == http.MethodPost && rel == "/sessions":
		s.handleOpenSession(w, r)
	case strings.HasPrefix(rel, "/sessions/"):
		s.handleSessionRoute(w, r, strings.TrimPrefix(rel, "/sessions/"))
	default:
		http.NotFound(w, r)
	}
}

type healthResponse struct {
	Mode           string                   `json:"mode"`
	Health         string                   `json:"health"`
	Closed         bool                     `json:"closed"`
	ActiveSessions int                      `json:"active_sessions"`
	Sessions       []harness.SessionSummary `json:"sessions,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	status := s.Service.Status()
	writeJSON(w, http.StatusOK, healthResponse{Mode: status.Mode, Health: status.Health, Closed: status.Closed, ActiveSessions: status.ActiveSessions, Sessions: status.Sessions})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.Service.Sessions()})
}

type openSessionRequest struct {
	Name      string `json:"name,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	StoreDir  string `json:"store_dir,omitempty"`
	Resume    string `json:"resume,omitempty"`
}

func (s *Server) handleOpenSession(w http.ResponseWriter, r *http.Request) {
	var req openSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, err := s.Service.OpenSession(r.Context(), harness.SessionOpenRequest{Name: req.Name, AgentName: req.AgentName, StoreDir: req.StoreDir, Resume: req.Resume})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session": sessionSummary(session)})
}

func (s *Server) handleSessionRoute(w http.ResponseWriter, r *http.Request, rel string) {
	parts := splitPath(rel)
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	session, ok := s.findSession(parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("session %q not found", parts[0]))
		return
	}
	switch {
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "commands":
		s.handleExecuteCommand(w, r, session)
	case r.Method == http.MethodGet && len(parts) == 2 && parts[1] == "events":
		s.handleEvents(w, r, session)
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "workflows" && parts[3] == "start":
		s.handleStartWorkflow(w, r, session, parts[2])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "workflows" && parts[2] == "runs":
		s.handleWorkflowRuns(w, r, session)
	default:
		http.NotFound(w, r)
	}
}

type commandRequest struct {
	Path  []string       `json:"path"`
	Input map[string]any `json:"input,omitempty"`
}

type commandResponse struct {
	Kind     string `json:"kind"`
	Payload  any    `json:"payload,omitempty"`
	Display  string `json:"display,omitempty"`
	JSONText string `json:"json,omitempty"`
}

func (s *Server) handleExecuteCommand(w http.ResponseWriter, r *http.Request, session *harness.Session) {
	var req commandRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := session.ExecuteCommand(r.Context(), req.Path, req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response, err := commandResultResponse(result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type workflowStartRequest struct {
	Input any  `json:"input,omitempty"`
	Async bool `json:"async,omitempty"`
}

func (s *Server) handleStartWorkflow(w http.ResponseWriter, r *http.Request, session *harness.Session, name string) {
	var req workflowStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Async {
		runID := session.StartWorkflow(r.Context(), name, req.Input)
		writeJSON(w, http.StatusAccepted, map[string]any{"run_id": runID, "status": "queued"})
		return
	}
	result := session.ExecuteWorkflow(r.Context(), name, req.Input)
	if result.Error != nil {
		writeError(w, http.StatusBadRequest, result.Error.Error())
		return
	}
	writeJSON(w, http.StatusOK, actionResultResponse(result))
}

func (s *Server) handleWorkflowRuns(w http.ResponseWriter, r *http.Request, session *harness.Session) {
	runs, ok, err := session.WorkflowRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "workflow runs require a thread-backed session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, session *harness.Session) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	events, cancel := session.Subscribe(16)
	defer cancel()
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSE(w, "session", eventResponse(event)); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleAGUIInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol": "ag-ui",
		"status":   "planned_compatibility_adapter",
		"native":   nativePrefix,
		"notes": []string{
			"AG-UI compatibility is intentionally namespaced away from the native Agents SDK API.",
			"Harness/session core stays protocol-neutral; AG-UI event mapping belongs at the channel boundary.",
			"A2UI is treated as a future generative UI payload format, not the runtime transport API.",
		},
	})
}

func (s *Server) findSession(idOrName string) (*harness.Session, bool) {
	if session, ok := s.Service.Session(idOrName); ok {
		return session, true
	}
	for _, summary := range s.Service.Sessions() {
		if summary.SessionID == idOrName {
			return s.Service.Session(summary.Name)
		}
	}
	return nil, false
}

func sessionSummary(session *harness.Session) harness.SessionSummary {
	info := session.Info()
	return harness.SessionSummary{Name: session.Name, SessionID: info.SessionID, AgentName: info.AgentName, ThreadBacked: info.ThreadBacked}
}

func commandResultResponse(result command.Result) (commandResponse, error) {
	response := commandResponse{Kind: commandResultKind(result.Kind), Payload: result.Payload}
	terminal, err := command.Render(result, command.DisplayTerminal)
	if err != nil {
		return response, err
	}
	jsonText, err := command.Render(result, command.DisplayJSON)
	if err != nil {
		return response, err
	}
	response.Display = terminal
	response.JSONText = jsonText
	return response, nil
}

func commandResultKind(kind command.ResultKind) string {
	switch kind {
	case command.ResultHandled:
		return "handled"
	case command.ResultDisplay:
		return "display"
	case command.ResultAgentTurn:
		return "agent_turn"
	case command.ResultReset:
		return "reset"
	case command.ResultExit:
		return "exit"
	default:
		return "unknown"
	}
}

type actionResponse struct {
	Status string `json:"status,omitempty"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

func actionResultResponse(result action.Result) actionResponse {
	response := actionResponse{Status: string(result.Status), Data: result.Data}
	if result.Error != nil {
		response.Error = result.Error.Error()
	}
	return response
}

type sessionEventResponse struct {
	Type           harness.SessionEventType `json:"type"`
	SessionName    string                   `json:"session_name,omitempty"`
	SessionID      string                   `json:"session_id,omitempty"`
	AgentName      string                   `json:"agent_name,omitempty"`
	Input          string                   `json:"input,omitempty"`
	CommandPath    []string                 `json:"command_path,omitempty"`
	WorkflowName   string                   `json:"workflow_name,omitempty"`
	CommandResult  *commandResponse         `json:"command_result,omitempty"`
	WorkflowResult *actionResponse          `json:"workflow_result,omitempty"`
	Error          string                   `json:"error,omitempty"`
}

func eventResponse(event harness.SessionEvent) sessionEventResponse {
	response := sessionEventResponse{Type: event.Type, SessionName: event.SessionName, SessionID: event.SessionID, AgentName: event.AgentName, Input: event.Input, CommandPath: event.CommandPath, WorkflowName: event.WorkflowName}
	if event.CommandResult.Kind != 0 || event.CommandResult.Payload != nil {
		if cmd, err := commandResultResponse(event.CommandResult); err == nil {
			response.CommandResult = &cmd
		}
	}
	if event.WorkflowResult.Data != nil || event.WorkflowResult.Error != nil || event.WorkflowResult.Status != "" {
		wf := actionResultResponse(event.WorkflowResult)
		response.WorkflowResult = &wf
	}
	if event.Error != nil {
		response.Error = event.Error.Error()
	}
	return response
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func splitPath(path string) []string {
	fields := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func writeSSE(w http.ResponseWriter, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}
