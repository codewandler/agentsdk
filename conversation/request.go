package conversation

import "github.com/codewandler/llmadapter/unified"

type Request struct {
	Model           string
	MaxOutputTokens *int
	Temperature     *float64
	Instructions    []unified.Instruction
	Tools           []unified.Tool
	ToolChoice      *unified.ToolChoice
	Messages        []unified.Message
	Stream          bool
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
func (b *Builder) Extension(key string, value any) *Builder {
	_ = b.req.Extensions.Set(key, value)
	return b
}
func (b *Builder) Build() Request { return b.req }
