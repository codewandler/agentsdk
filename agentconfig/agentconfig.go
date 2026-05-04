// Package agentconfig defines agent specification and configuration types that
// are independent of a running agent instance. These types are used by resource
// loaders, CLI flag parsing, harness session configuration, and other packages
// that need to describe agent configuration without importing the live runtime.
//
// The [agent] package re-exports all types from this package so existing
// callers continue to work. New code that only needs config/spec types should
// import agentconfig directly to avoid pulling in the agent runtime dependency
// graph.
package agentconfig

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/codewandler/llmadapter/unified"
)

// Spec describes an agent identity/configuration independent of a running
// conversation session.
type Spec struct {
	Name              string
	Description       string
	System            string
	Inference         InferenceOptions
	MaxSteps          int
	Tools             []string
	Skills            []string
	SkillSources      []skill.Source
	Commands          []string
	InstructionPaths  []string
	ResourceID        string
	ResourceFrom      string
	Capabilities      []capability.AttachSpec
	AutoCompaction    AutoCompactionConfig
	AutoCompactionSet bool
	HasFrontmatter    bool
}

// ThinkingMode controls extended thinking behavior.
type ThinkingMode string

const (
	ThinkingModeAuto ThinkingMode = "auto"
	ThinkingModeOn   ThinkingMode = "on"
	ThinkingModeOff  ThinkingMode = "off"
)

// InferenceOptions holds model request parameters.
type InferenceOptions struct {
	Model       string
	MaxTokens   int
	Thinking    ThinkingMode
	Effort      unified.ReasoningEffort
	Temperature float64
}

// DefaultInferenceOptions returns conservative defaults for a terminal agent.
func DefaultInferenceOptions() InferenceOptions {
	return InferenceOptions{
		Model:       "codex/gpt-5.5",
		MaxTokens:   16_000,
		Thinking:    ThinkingModeAuto,
		Effort:      unified.ReasoningEffortMedium,
		Temperature: 0.1,
	}
}

// AutoCompactionConfig controls automatic compaction between turns.
type AutoCompactionConfig struct {
	Enabled            bool
	ContextWindowRatio float64 // fraction of model context window; 0 = default (0.85)
	KeepWindow         int     // messages to preserve; 0 = default (4)

	// Deprecated: absolute thresholds are no longer used. Configure
	// ContextWindowRatio instead so compaction follows modeldb context-window
	// metadata.
	TokenThreshold int
}

// ModelUseCase identifies a compatibility use case for model routing.
type ModelUseCase string

const (
	ModelUseCaseAgenticCoding ModelUseCase = "agentic_coding"
	ModelUseCaseSummarization ModelUseCase = "summarization"
)

// ModelPolicy configures model compatibility routing.
type ModelPolicy struct {
	UseCase       ModelUseCase
	SourceAPI     adapt.ApiKind
	ApprovedOnly  bool
	AllowDegraded bool
	AllowUntested bool
	EvidencePath  string
}

// Configured reports whether any policy field is set.
func (p ModelPolicy) Configured() bool {
	return p.UseCase != "" ||
		p.SourceAPI != "" ||
		p.ApprovedOnly ||
		p.AllowDegraded ||
		p.AllowUntested ||
		p.EvidencePath != ""
}

// LLMUseCase converts the policy use case to the llmadapter compatibility type.
func (p ModelPolicy) LLMUseCase() (compatibility.UseCase, error) {
	if p.UseCase == "" {
		if p.ApprovedOnly {
			return compatibility.UseCaseAgenticCoding, nil
		}
		return "", nil
	}
	return compatibility.ParseUseCase(string(p.UseCase))
}

// ParseModelUseCase parses a string into a ModelUseCase, validating it against
// known llmadapter use cases.
func ParseModelUseCase(value string) (ModelUseCase, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	useCase, err := compatibility.ParseUseCase(value)
	if err != nil {
		return "", err
	}
	return ModelUseCase(useCase), nil
}

// ParseSourceAPI parses a source API string into an adapt.ApiKind.
func ParseSourceAPI(value string) (adapt.ApiKind, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "auto":
		return "", nil
	case string(adapt.ApiOpenAIResponses):
		return adapt.ApiOpenAIResponses, nil
	case string(adapt.ApiOpenAIChatCompletions), "openai.chat.completions":
		return adapt.ApiOpenAIChatCompletions, nil
	case string(adapt.ApiAnthropicMessages):
		return adapt.ApiAnthropicMessages, nil
	default:
		return "", fmt.Errorf("unknown source api %q", value)
	}
}

// FormatSourceAPI formats an adapt.ApiKind for display.
func FormatSourceAPI(api adapt.ApiKind) string {
	if api == "" {
		return "auto"
	}
	return string(api)
}
