package conversation

import "github.com/codewandler/llmadapter/unified"

type Request struct {
	Model           string
	MaxOutputTokens *int
	Temperature     *float64
	TopP            *float64
	TopK            *int
	Stop            []string
	Seed            *int64
	ResponseFormat  *unified.ResponseFormat
	Reasoning       *unified.ReasoningConfig
	Safety          *unified.SafetyConfig
	Instructions    []unified.Instruction
	Tools           []unified.Tool
	ToolChoice      *unified.ToolChoice
	Messages        []unified.Message
	Stream          bool
	User            string
	Extensions      unified.Extensions
}

type Builder struct {
	req Request
}

func NewRequest() *Builder { return &Builder{} }

func (b *Builder) Model(model string) *Builder { b.req.Model = model; return b }
func (b *Builder) MaxOutputTokens(max int) *Builder {
	b.req.MaxOutputTokens = &max
	return b
}
func (b *Builder) Temperature(v float64) *Builder {
	b.req.Temperature = &v
	return b
}
func (b *Builder) TopP(v float64) *Builder {
	b.req.TopP = &v
	return b
}
func (b *Builder) TopK(v int) *Builder {
	b.req.TopK = &v
	return b
}
func (b *Builder) Stop(stop ...string) *Builder {
	b.req.Stop = append(b.req.Stop, stop...)
	return b
}
func (b *Builder) Seed(seed int64) *Builder {
	b.req.Seed = &seed
	return b
}
func (b *Builder) ResponseFormat(format unified.ResponseFormat) *Builder {
	b.req.ResponseFormat = &format
	return b
}
func (b *Builder) Reasoning(reasoning unified.ReasoningConfig) *Builder {
	b.req.Reasoning = &reasoning
	return b
}
func (b *Builder) Safety(safety unified.SafetyConfig) *Builder {
	b.req.Safety = &safety
	return b
}
func (b *Builder) Instructions(instructions ...unified.Instruction) *Builder {
	b.req.Instructions = append(b.req.Instructions, instructions...)
	return b
}
func (b *Builder) Tools(tools []unified.Tool) *Builder {
	b.req.Tools = append([]unified.Tool(nil), tools...)
	return b
}
func (b *Builder) ToolChoice(choice unified.ToolChoice) *Builder {
	b.req.ToolChoice = &choice
	return b
}
func (b *Builder) Message(msg unified.Message) *Builder {
	b.req.Messages = append(b.req.Messages, msg)
	return b
}
func (b *Builder) User(text string) *Builder {
	return b.Message(unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: text}}})
}
func (b *Builder) ToolResult(callID, name, output string, isError bool) *Builder {
	return b.Message(unified.Message{
		Role: unified.RoleTool,
		ToolResults: []unified.ToolResult{{
			ToolCallID: callID,
			Name:       name,
			Content:    []unified.ContentPart{unified.TextPart{Text: output}},
			IsError:    isError,
		}},
	})
}
func (b *Builder) Stream(stream bool) *Builder { b.req.Stream = stream; return b }
func (b *Builder) UserID(user string) *Builder { b.req.User = user; return b }
func (b *Builder) Extension(key string, value any) *Builder {
	_ = b.req.Extensions.Set(key, value)
	return b
}
func (b *Builder) Build() Request { return b.req }
