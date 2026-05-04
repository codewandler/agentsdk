// Package harness provides the first named host/session seam over the current
// app and agent runtime stack. It intentionally delegates to app.App for now;
// later slices can move lifecycle-heavy responsibilities behind this boundary.
package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/thread"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/trigger"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/agentsdk/workflow"
)

type Service struct {
	App *app.App

	mu       sync.Mutex
	sessions map[string]*Session
	closed   bool
	triggers trigger.RegistryView
}

type Session struct {
	App    *app.App
	Agent  *agent.Instance
	Name   string
	turnID int

	service         *Service
	threadStore     thread.Store
	storeDir        string
	mu              sync.Mutex
	closed          bool
	nextSub         int
	subs            map[int]chan SessionEvent
	workflowCancels map[workflow.RunID]context.CancelFunc
}

// SessionOpenRequest describes a harness-owned session open/resume operation.
// AgentName defaults to the app default agent when empty. StoreDir enables
// thread-backed persistence; Resume may be either a session ID or JSONL path.
type SessionOpenRequest struct {
	Name         string
	AgentName    string
	StoreDir     string
	Resume       string
	AgentOptions []agent.Option
}

type SessionSummary struct {
	Name         string
	SessionID    string
	AgentName    string
	ThreadBacked bool
	Closed       bool
}

// ServiceStatus is a stable, side-effect-free snapshot for long-running hosts
// such as daemon processes and HTTP/SSE control planes.
type ServiceStatus struct {
	Mode           string
	Health         string
	Closed         bool
	ActiveSessions int
	Sessions       []SessionSummary
}
type SessionEventType string

const (
	SessionEventOpened     SessionEventType = "opened"
	SessionEventInput      SessionEventType = "input"
	SessionEventCommand    SessionEventType = "command"
	SessionEventWorkflow   SessionEventType = "workflow"
	SessionEventCompaction SessionEventType = "compaction"
	SessionEventClosed     SessionEventType = "closed"
)

type SessionEvent struct {
	Type            SessionEventType
	SessionName     string
	SessionID       string
	AgentName       string
	Input           string
	CommandPath     []string
	WorkflowName    string
	CommandResult   command.Result
	WorkflowResult  action.Result
	CompactionEvent agent.CompactionEvent
	Error           error
}

func NewService(app *app.App) *Service {
	return &Service{App: app, sessions: map[string]*Session{}}
}


// OpenSession instantiates an agent and registers it as an active harness
// session. It is the stable API for opening named sessions.
func (s *Service) OpenSession(ctx context.Context, req SessionOpenRequest) (*Session, error) {
	if s == nil || s.App == nil {
		return nil, fmt.Errorf("harness: app is required")
	}
	if s.isClosed() {
		return nil, fmt.Errorf("harness: service is closed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Open the thread store at the harness level so the session owns the
	// store reference. The agent receives the pre-opened store via
	// WithThreadStore and no longer needs to know about JSONL paths.
	var (
		threadStore thread.Store
		storeDir    string
	)
	if req.StoreDir != "" {
		storeDir = req.StoreDir
		threadStore = threadjsonlstore.Open(req.StoreDir)
	}

	// When the harness owns the store, parse the resume ref to extract the
	// session ID. The agent should receive a plain ID, not a filesystem path,
	// because the store is already open.
	resumeID := req.Resume
	if threadStore != nil && resumeID != "" {
		resumeID = parseSessionID(resumeID)
	}

	opts := append([]agent.Option(nil), req.AgentOptions...)
	if threadStore != nil {
		opts = append(opts, agent.WithThreadStore(threadStore))
	}
	if req.StoreDir != "" {
		opts = append(opts, agent.WithSessionStoreDir(req.StoreDir))
	}
	if resumeID != "" {
		opts = append(opts, agent.WithResumeSession(resumeID))
	}
	var (
		inst *agent.Instance
		err  error
	)
	if strings.TrimSpace(req.AgentName) == "" {
		inst, err = s.App.InstantiateDefaultAgent(opts...)
	} else {
		inst, err = s.App.InstantiateAgent(req.AgentName, opts...)
	}
	if err != nil {
		return nil, err
	}
	return s.attachSession(req.Name, inst, threadStore, storeDir)
}

// ResumeSession is the stable API for resuming an existing persisted session by
// ID or JSONL path. It requires Resume and otherwise follows OpenSession.
func (s *Service) ResumeSession(ctx context.Context, req SessionOpenRequest) (*Session, error) {
	if strings.TrimSpace(req.Resume) == "" {
		return nil, fmt.Errorf("harness: resume session is required")
	}
	return s.OpenSession(ctx, req)
}

func (s *Service) attachSession(name string, inst *agent.Instance, store thread.Store, storeDir string) (*Session, error) {
	if s == nil || s.App == nil {
		return nil, fmt.Errorf("harness: app is required")
	}
	if inst == nil {
		return nil, fmt.Errorf("harness: no agent configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("harness: service is closed")
	}
	if strings.TrimSpace(name) == "" {
		name = inst.SessionID()
	}
	// If the caller didn't provide a store, try to get it from the agent
	// (backward compat for code that uses agent.WithSessionStoreDir directly).
	if store == nil {
		store = inst.ThreadStore()
	}
	session := &Session{App: s.App, Agent: inst, Name: name, service: s, threadStore: store, storeDir: storeDir, subs: map[int]chan SessionEvent{}, workflowCancels: map[workflow.RunID]context.CancelFunc{}}
	inst.AddCompactionEventHandler(func(event agent.CompactionEvent) {
		session.publish(SessionEvent{Type: SessionEventCompaction, CompactionEvent: event})
	})
	if err := session.AttachAgentProjection(session.AgentCommandProjection()); err != nil {
		return nil, err
	}
	if s.sessions == nil {
		s.sessions = map[string]*Session{}
	}
	s.sessions[name] = session
	session.publish(SessionEvent{Type: SessionEventOpened})
	return session, nil
}

func (s *Service) Sessions() []SessionSummary {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	sessions := make(map[string]*Session, len(s.sessions))
	for name, session := range s.sessions {
		sessions[name] = session
	}
	s.mu.Unlock()
	names := make([]string, 0, len(sessions))
	for name := range sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]SessionSummary, 0, len(names))
	for _, name := range names {
		out = append(out, sessions[name].summary())
	}
	return out
}

// Session returns an active session by registry name. The boolean is false when
// the service is nil, closed, or the name is not currently registered.
func (s *Service) Session(name string) (*Session, bool) {
	if s == nil || strings.TrimSpace(name) == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false
	}
	session, ok := s.sessions[name]
	return session, ok
}

// Status returns a stable health/registry snapshot suitable for long-running
// host control planes. It does not open, resume, or close sessions.
func (s *Service) Status() ServiceStatus {
	closed := s.isClosed()
	sessions := s.Sessions()
	health := "ok"
	if closed {
		health = "closed"
	}
	return ServiceStatus{
		Mode:           "harness.service",
		Health:         health,
		Closed:         closed,
		ActiveSessions: len(sessions),
		Sessions:       sessions,
	}
}

func (s *Service) SetTriggerRegistry(registry trigger.RegistryView) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.triggers = registry
	s.mu.Unlock()
}

func (s *Service) TriggerRegistry() trigger.RegistryView {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggers
}
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.sessions = map[string]*Session{}
	s.closed = true
	s.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
	return nil
}

func (s *Service) isClosed() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Session) Send(ctx context.Context, input string) (command.Result, error) {
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	defer func() { s.publish(SessionEvent{Type: SessionEventInput, Input: input}) }()
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return command.Handled(), nil
	}
	if strings.HasPrefix(trimmed, "/") {
		name, params, err := command.Parse(trimmed)
		if err != nil {
			return command.Result{}, err
		}
		commands, err := s.Commands()
		if err != nil {
			return command.Result{}, err
		}
		if cmd, ok := commands.Get(name); ok {
			result, err := cmd.Execute(ctx, params)
			if err != nil {
				return command.Result{}, err
			}
			return s.applyResult(ctx, result, 0)
		}
		if s.App.Commands() != nil {
			result, err := s.App.Commands().ExecuteUser(ctx, trimmed)
			if err != nil {
				return command.Result{}, err
			}
			return s.applyResult(ctx, result, 0)
		}
	}
	return s.runAgentTurn(ctx, trimmed, 0)
}

func (s *Session) applyResult(ctx context.Context, result command.Result, turnID int) (command.Result, error) {
	switch result.Kind {
	case command.ResultAgentTurn:
		input, ok := command.AgentTurnInput(result)
		if !ok || strings.TrimSpace(input) == "" {
			return command.Handled(), nil
		}
		return s.runAgentTurn(ctx, input, turnID)
	case command.ResultReset:
		if s != nil && s.Agent != nil {
			s.Agent.Reset()
		}
		if s != nil {
			s.turnID = 0
		}
		return command.Handled(), nil
	default:
		return result, nil
	}
}

func (s *Session) runAgentTurn(ctx context.Context, input string, turnID int) (command.Result, error) {
	if s == nil || s.Agent == nil {
		return command.Result{}, fmt.Errorf("harness: no agent configured")
	}
	if turnID <= 0 {
		s.turnID++
		turnID = s.turnID
	}
	return command.Handled(), s.Agent.RunTurn(ctx, turnID, input)
}

func (s *Session) Commands() (*command.Registry, error) {
	builders := []func(*Session) (*command.Tree, error){
		newHelpCommand,
		newAgentsCommand,
		newNewCommand,
		newQuitCommand,
		newTurnCommand,
		newContextCommand,
		newCapabilitiesCommand,
		newSkillsCommand,
		newSkillCommand,
		newCompactCommand,
		newWorkflowCommand,
		newJobsCommand,
		newSessionCommand,
	}
	registry := command.NewRegistry()
	for _, build := range builders {
		tree, err := build(s)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(tree); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (s *Session) ExecuteCommand(ctx context.Context, path []string, input map[string]any) (command.Result, error) {
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	commands, err := s.Commands()
	if err != nil {
		return command.Result{}, err
	}
	result, err := commands.ExecuteMap(ctx, path, input)
	s.publish(SessionEvent{Type: SessionEventCommand, CommandPath: append([]string(nil), path...), CommandResult: result, Error: err})
	return result, err
}

func (s *Session) ExecuteWorkflow(ctx context.Context, workflowName string, input any, opts ...workflow.ExecuteOption) action.Result {
	if s == nil || s.App == nil {
		return action.Result{Error: fmt.Errorf("harness: app is required")}
	}
	execOpts, recorder := s.workflowExecutionOptions(workflowName, input, opts)
	result := s.App.ExecuteWorkflow(ctx, workflowName, input, execOpts...)
	if recorder != nil {
		result.Error = errors.Join(result.Error, recorder.Err())
	}
	s.publish(SessionEvent{Type: SessionEventWorkflow, WorkflowName: workflowName, WorkflowResult: result, Error: result.Error})
	return result
}

func (s *Session) StartWorkflow(ctx context.Context, workflowName string, input any, opts ...workflow.ExecuteOption) workflow.RunID {
	runID := workflow.NewRunID()
	s.StartWorkflowWithRunID(ctx, runID, workflowName, input, opts...)
	return runID
}

func (s *Session) StartWorkflowWithRunID(ctx context.Context, runID workflow.RunID, workflowName string, input any, opts ...workflow.ExecuteOption) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store, ok := s.WorkflowRunStore(); ok {
		_ = store.Append(ctx, runID, workflow.Queued{RunID: runID, WorkflowName: workflowName, Metadata: s.WorkflowRunMetadata("command", []string{"workflow", "start"}), Input: workflow.InlineValue(input)})
	}
	// Async workflow runs must outlive the command/REPL line context that started
	// them. Cancellation is owned by Session.CancelWorkflow and service/session
	// shutdown rather than by the short-lived caller context.
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.workflowCancels == nil {
		s.workflowCancels = map[workflow.RunID]context.CancelFunc{}
	}
	s.workflowCancels[runID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.workflowCancels, runID)
			s.mu.Unlock()
		}()
		s.ExecuteWorkflow(runCtx, workflowName, input, append(opts, workflow.WithRunID(runID))...)
	}()
}

func (s *Session) CancelWorkflow(ctx context.Context, runID workflow.RunID, reason string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	cancel := s.workflowCancels[runID]
	if cancel != nil {
		delete(s.workflowCancels, runID)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow run %q not found", runID)
	}
	if state.Status == workflow.RunSucceeded || state.Status == workflow.RunFailed || state.Status == workflow.RunCanceled {
		return nil
	}
	store, hasStore := s.WorkflowRunStore()
	if !hasStore {
		return fmt.Errorf("workflow runs require a thread-backed session")
	}
	if reason == "" {
		reason = "canceled"
	}
	return store.Append(ctx, runID, workflow.Canceled{RunID: runID, WorkflowName: state.WorkflowName, Reason: reason})
}

func (s *Session) WorkflowRunMetadata(trigger string, commandPath []string) workflow.RunMetadata {
	info := s.Info()
	metadata := workflow.RunMetadata{SessionID: info.SessionID, AgentName: info.AgentName, ThreadID: string(info.ThreadID), BranchID: string(info.BranchID), Trigger: trigger}
	metadata.CommandPath = append([]string(nil), commandPath...)
	return metadata
}

func (s *Session) workflowExecutionOptions(workflowName string, input any, opts []workflow.ExecuteOption) ([]workflow.ExecuteOption, *workflow.ThreadRecorder) {
	out := []workflow.ExecuteOption{workflow.WithRunMetadata(s.WorkflowRunMetadata("harness", []string{"workflow", "start"})), workflow.WithInputRef(workflow.InlineValue(input))}
	if s != nil && s.App != nil {
		if def, ok := s.App.Workflow(workflowName); ok {
			out = append(out, workflow.WithDefinitionIdentity(workflow.DefinitionHash(def), def.Version))
		}
	}
	out = append(out, opts...)
	if s == nil || s.Agent == nil || s.Agent.LiveThread() == nil {
		return out, nil
	}
	recorder := &workflow.ThreadRecorder{Live: s.Agent.LiveThread()}
	out = append(out, workflow.WithEventHandler(recorder.OnEvent))
	return out, recorder
}

func (s *Session) WorkflowRunStore() (*workflow.ThreadRunStore, bool) {
	if s == nil || s.Agent == nil {
		return nil, false
	}
	live := s.Agent.LiveThread()
	if live == nil {
		return nil, false
	}
	store := s.resolveStore()
	if store == nil {
		return nil, false
	}
	return &workflow.ThreadRunStore{Store: store, Live: live, ThreadID: live.ID(), BranchID: live.BranchID()}, true
}

// ThreadEvents returns the persisted events for the session's live thread and
// active branch. It is intended for harness/channel inspection and replay tests;
// callers should treat the returned events as immutable snapshots.
func (s *Session) ThreadEvents(ctx context.Context) ([]thread.Event, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if s == nil || s.Agent == nil || s.Agent.LiveThread() == nil {
		return nil, false, nil
	}
	store := s.resolveStore()
	if store == nil {
		return nil, false, nil
	}
	live := s.Agent.LiveThread()
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, false, err
	}
	events, err := stored.EventsForBranch(live.BranchID())
	if err != nil {
		return nil, false, err
	}
	return events, true, nil
}

func (s *Session) WorkflowRunState(ctx context.Context, runID workflow.RunID) (workflow.RunState, bool, error) {
	store, ok := s.WorkflowRunStore()
	if !ok {
		return workflow.RunState{}, false, nil
	}
	return store.State(ctx, runID)
}
func (s *Session) WorkflowRunEvents(ctx context.Context, runID workflow.RunID) ([]any, bool, error) {
	store, ok := s.WorkflowRunStore()
	if !ok {
		return nil, false, nil
	}
	return store.Events(ctx, runID)
}

func (s *Session) WorkflowRuns(ctx context.Context) ([]workflow.RunSummary, bool, error) {
	store, ok := s.WorkflowRunStore()
	if !ok {
		return nil, false, nil
	}
	summaries, err := store.Runs(ctx)
	if err != nil {
		return nil, true, err
	}
	return summaries, true, nil
}

func (s *Session) Info() SessionInfo {
	info := SessionInfo{}
	if s == nil {
		return info
	}
	if s.Agent != nil {
		spec := s.Agent.Spec()
		info.AgentName = spec.Name
		info.SessionID = s.Agent.SessionID()
		info.ParamsSummary = s.Agent.ParamsSummary()
		if live := s.Agent.LiveThread(); live != nil {
			info.ThreadID = live.ID()
			info.BranchID = live.BranchID()
			info.ThreadBacked = true
		}
	}
	return info
}

func (s *Session) ContextState() ContextState {
	state := ContextState{Text: "context: unavailable"}
	if s == nil || s.Agent == nil {
		return state
	}
	state.Agent = s.Agent.Spec().Name
	state.Text = s.Agent.ContextState()
	state.Descriptors = s.Agent.ContextDescriptors()
	state.Snapshot = s.Agent.ContextSnapshot()
	return state
}

func (s *Session) CapabilityState() CapabilityState {
	state := CapabilityState{}
	if s == nil || s.Agent == nil {
		return state
	}
	state.Agent = s.Agent.Spec().Name
	state.Capabilities = s.Agent.CapabilityDescriptors()
	return state
}

func (s *Session) CompactionPolicy() agent.CompactionPolicy {
	if s == nil || s.Agent == nil {
		return agent.CompactionPolicy{}
	}
	return s.Agent.CompactionPolicy()
}

func (s *Session) ParamsSummary() string {
	if s == nil || s.Agent == nil {
		return ""
	}
	return s.Agent.ParamsSummary()
}

func (s *Session) SessionID() string {
	return s.Info().SessionID
}

// resolveStore returns the session's thread store. It prefers the harness-owned
// store, falling back to the agent's store for backward compatibility.
func (s *Session) resolveStore() thread.Store {
	if s.threadStore != nil {
		return s.threadStore
	}
	if s.Agent != nil {
		return s.Agent.ThreadStore()
	}
	return nil
}

func (s *Session) Tracker() *usage.Tracker {
	if s == nil || s.Agent == nil {
		return nil
	}
	return s.Agent.Tracker()
}

func (s *Session) Out() io.Writer {
	if s == nil || s.Agent == nil {
		return io.Discard
	}
	return s.Agent.Out()
}

func (s *Session) Subscribe(buffer int) (<-chan SessionEvent, func()) {
	if buffer < 0 {
		buffer = 0
	}
	ch := make(chan SessionEvent, buffer)
	if s == nil {
		close(ch)
		return ch, func() {}
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	if s.subs == nil {
		s.subs = map[int]chan SessionEvent{}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		if sub, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(sub)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()

	if svc := s.service; svc != nil {
		svc.mu.Lock()
		if svc.sessions != nil && svc.sessions[s.Name] == s {
			delete(svc.sessions, s.Name)
		}
		svc.mu.Unlock()
	}

	event := s.enrichEvent(SessionEvent{Type: SessionEventClosed})
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
		close(ch)
	}
	return nil
}

func (s *Session) TriggerRegistry() trigger.RegistryView {
	if s == nil || s.service == nil {
		return nil
	}
	return s.service.TriggerRegistry()
}

func (s *Session) summary() SessionSummary {
	if s == nil {
		return SessionSummary{}
	}
	info := s.Info()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	return SessionSummary{Name: s.Name, SessionID: info.SessionID, AgentName: info.AgentName, ThreadBacked: info.ThreadBacked, Closed: closed}
}

func (s *Session) publish(event SessionEvent) {
	if s == nil {
		return
	}
	event = s.enrichEvent(event)
	s.mu.Lock()
	if s.closed || len(s.subs) == 0 {
		s.mu.Unlock()
		return
	}
	subs := make([]chan SessionEvent, 0, len(s.subs))
	for _, ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Session) enrichEvent(event SessionEvent) SessionEvent {
	if s == nil {
		return event
	}
	info := s.Info()
	event.SessionName = s.Name
	event.SessionID = info.SessionID
	event.AgentName = info.AgentName
	return event
}

// parseSessionID extracts a plain session ID from a resume reference that may
// be a bare ID ("b6mRRINc") or a full JSONL path ("/tmp/sessions/b6mRRINc.jsonl").
// This mirrors the parsing previously done inside agent.splitThreadSessionRef.
func parseSessionID(ref string) string {
	cleaned := strings.TrimSpace(ref)
	if cleaned == "" {
		return ""
	}
	if filepath.Ext(cleaned) == ".jsonl" || strings.Contains(cleaned, string(os.PathSeparator)) {
		return strings.TrimSuffix(filepath.Base(cleaned), filepath.Ext(cleaned))
	}
	return cleaned
}
