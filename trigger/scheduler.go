package trigger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type JobStatus string

const (
	JobStarting JobStatus = "starting"
	JobActive   JobStatus = "active"
	JobRunning  JobStatus = "running"
	JobSkipped  JobStatus = "skipped"
	JobStopped  JobStatus = "stopped"
	JobFailed   JobStatus = "failed"
)

type JobEventType string

const (
	JobEventStarted   JobEventType = "started"
	JobEventMatched   JobEventType = "matched"
	JobEventSkipped   JobEventType = "skipped"
	JobEventCompleted JobEventType = "completed"
	JobEventFailed    JobEventType = "failed"
	JobEventStopped   JobEventType = "stopped"
)

type JobEvent struct {
	Type   JobEventType
	RuleID RuleID
	Event  Event
	Status JobStatus
	Result ExecutionResult
	Error  string
	At     time.Time
}

type JobSummary struct {
	RuleID          RuleID
	SourceID        SourceID
	EventType       string
	TargetKind      TargetKind
	TargetName      string
	SessionMode     SessionMode
	SessionName     string
	Status          JobStatus
	Running         bool
	LastEventID     EventID
	LastFire        time.Time
	LastError       string
	LastWorkflowRun string
	Matched         int
	Skipped         int
}

type RegistryView interface {
	Jobs() []JobSummary
	StopJob(RuleID) error
}

type Scheduler struct {
	executor Executor
	now      func() time.Time

	mu     sync.Mutex
	jobs   map[RuleID]*job
	events []chan JobEvent
}

type SchedulerOption func(*Scheduler)

func WithNow(now func() time.Time) SchedulerOption {
	return func(s *Scheduler) { s.now = now }
}

func NewScheduler(executor Executor, opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{executor: executor, now: time.Now, jobs: map[RuleID]*job{}}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *Scheduler) AddRule(ctx context.Context, rule Rule) error {
	if s == nil {
		return fmt.Errorf("trigger: scheduler is required")
	}
	if s.executor == nil {
		return fmt.Errorf("trigger: executor is required")
	}
	rule = rule.Normalized()
	if rule.ID == "" {
		return fmt.Errorf("trigger: rule id is required")
	}
	if rule.Source == nil {
		return fmt.Errorf("trigger: rule %q source is required", rule.ID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	jobCtx, cancel := context.WithCancel(ctx)
	j := &job{rule: rule, cancel: cancel, status: JobStarting}
	s.mu.Lock()
	if _, exists := s.jobs[rule.ID]; exists {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("trigger: job %q already exists", rule.ID)
	}
	s.jobs[rule.ID] = j
	s.mu.Unlock()
	s.publish(JobEvent{Type: JobEventStarted, RuleID: rule.ID, Status: JobActive})
	go s.runSource(jobCtx, j)
	return nil
}

func (s *Scheduler) Subscribe(buffer int) (<-chan JobEvent, func()) {
	if buffer < 0 {
		buffer = 0
	}
	ch := make(chan JobEvent, buffer)
	if s == nil {
		close(ch)
		return ch, func() {}
	}
	s.mu.Lock()
	s.events = append(s.events, ch)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		for i, candidate := range s.events {
			if candidate == ch {
				s.events = append(s.events[:i], s.events[i+1:]...)
				close(ch)
				break
			}
		}
		s.mu.Unlock()
	}
}

func (s *Scheduler) Jobs() []JobSummary {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	jobs := make([]*job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.mu.Unlock()
	out := make([]JobSummary, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.summary())
	}
	return out
}

func (s *Scheduler) StopJob(id RuleID) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	j, ok := s.jobs[id]
	if ok {
		delete(s.jobs, id)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("trigger: job %q not found", id)
	}
	j.cancel()
	j.setStatus(JobStopped)
	s.publish(JobEvent{Type: JobEventStopped, RuleID: id, Status: JobStopped})
	return nil
}

func (s *Scheduler) StopAll() {
	for _, summary := range s.Jobs() {
		_ = s.StopJob(summary.RuleID)
	}
}

func (s *Scheduler) runSource(ctx context.Context, j *job) {
	j.setStatus(JobActive)
	err := j.rule.Source.Start(ctx, func(event Event) { s.handleEvent(ctx, j, event) })
	if err != nil && ctx.Err() == nil {
		j.setError(err)
		s.publish(JobEvent{Type: JobEventFailed, RuleID: j.rule.ID, Status: JobFailed, Error: err.Error()})
	}
}

func (s *Scheduler) handleEvent(ctx context.Context, j *job, event Event) {
	ok, err := j.rule.Matcher.Match(event)
	if err != nil {
		j.setError(err)
		s.publish(JobEvent{Type: JobEventFailed, RuleID: j.rule.ID, Event: event, Status: JobFailed, Error: err.Error()})
		return
	}
	if !ok {
		return
	}
	if !j.tryStartRun(event) {
		s.publish(JobEvent{Type: JobEventSkipped, RuleID: j.rule.ID, Event: event, Status: JobSkipped})
		return
	}
	s.publish(JobEvent{Type: JobEventMatched, RuleID: j.rule.ID, Event: event, Status: JobRunning})
	go func() {
		defer j.finishRun()
		runCtx := ctx
		if j.rule.Policy.Timeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(ctx, j.rule.Policy.Timeout)
			defer cancel()
		}
		result, err := s.executor.ExecuteTrigger(runCtx, Execution{Rule: j.rule, Event: event})
		if err != nil {
			j.setError(err)
			s.publish(JobEvent{Type: JobEventFailed, RuleID: j.rule.ID, Event: event, Status: JobFailed, Error: err.Error()})
			return
		}
		j.setResult(result)
		s.publish(JobEvent{Type: JobEventCompleted, RuleID: j.rule.ID, Event: event, Status: JobActive, Result: result})
	}()
}

func (s *Scheduler) publish(event JobEvent) {
	if s == nil {
		return
	}
	if event.At.IsZero() {
		event.At = s.now().UTC()
	}
	s.mu.Lock()
	subs := append([]chan JobEvent(nil), s.events...)
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

type job struct {
	rule   Rule
	cancel context.CancelFunc

	mu       sync.Mutex
	status   JobStatus
	running  bool
	last     Event
	lastFire time.Time
	lastErr  string
	lastRes  ExecutionResult
	matched  int
	skipped  int
}

func (j *job) tryStartRun(event Event) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running && j.rule.Policy.Overlap == OverlapSkipIfRunning {
		j.skipped++
		j.status = JobSkipped
		j.last = event
		j.lastFire = event.At
		return false
	}
	j.running = true
	j.status = JobRunning
	j.last = event
	j.lastFire = event.At
	j.matched++
	return true
}

func (j *job) finishRun() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.running = false
	if j.status != JobFailed {
		j.status = JobActive
	}
}

func (j *job) setStatus(status JobStatus) {
	j.mu.Lock()
	j.status = status
	j.mu.Unlock()
}

func (j *job) setError(err error) {
	j.mu.Lock()
	j.status = JobFailed
	j.lastErr = err.Error()
	j.mu.Unlock()
}

func (j *job) setResult(result ExecutionResult) {
	j.mu.Lock()
	j.lastRes = result
	j.lastErr = ""
	j.mu.Unlock()
}

func (j *job) summary() JobSummary {
	j.mu.Lock()
	defer j.mu.Unlock()
	return JobSummary{
		RuleID:          j.rule.ID,
		SourceID:        j.rule.Source.ID(),
		EventType:       j.rule.EventType(),
		TargetKind:      j.rule.Target.Kind,
		TargetName:      j.rule.Target.Name(),
		SessionMode:     j.rule.Session.Mode,
		SessionName:     j.rule.Session.Name,
		Status:          j.status,
		Running:         j.running,
		LastEventID:     j.last.ID,
		LastFire:        j.lastFire,
		LastError:       j.lastErr,
		LastWorkflowRun: j.lastRes.WorkflowRunID,
		Matched:         j.matched,
		Skipped:         j.skipped,
	}
}
