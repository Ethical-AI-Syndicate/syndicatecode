package models

import (
	"context"
	"encoding/json"
)

// ContentBlock is a sealed sum type for message content parts.
type ContentBlock interface{ contentBlock() }

// TextBlock is a plain text content part.
type TextBlock struct{ Text string }

func (TextBlock) contentBlock() {}

// ToolUseBlock represents a model-requested tool call.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolUseBlock) contentBlock() {}

// ToolResultBlock carries the result of a tool call back to the model.
type ToolResultBlock struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (ToolResultBlock) contentBlock() {}

// Message is a single turn in a conversation.
type Message struct {
	Role    string // "user" or "assistant"
	Content []ContentBlock
}

// Tool describes a function the model may invoke.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// Params configures a single model call.
type Params struct {
	Model     string
	Messages  []Message
	Tools     []Tool
	System    string
	MaxTokens int
}

// StreamEvent is a sealed sum type for streaming response chunks.
type StreamEvent interface{ streamEvent() }

// TextDeltaEvent carries a token of streamed text.
type TextDeltaEvent struct{ Delta string }

func (TextDeltaEvent) streamEvent() {}

// ToolUseStartEvent signals the start of a tool call with its ID and name.
type ToolUseStartEvent struct{ ID, Name string }

func (ToolUseStartEvent) streamEvent() {}

// ToolInputDeltaEvent carries a chunk of tool input JSON.
type ToolInputDeltaEvent struct{ ID, Delta string }

func (ToolInputDeltaEvent) streamEvent() {}

// MessageDeltaEvent carries stop reason and output token count at end of response.
type MessageDeltaEvent struct {
	OutputTokens int
	StopReason   string
}

func (MessageDeltaEvent) streamEvent() {}

// MessageStartEvent carries input token count at the beginning of a response.
type MessageStartEvent struct{ InputTokens int }

func (MessageStartEvent) streamEvent() {}

// LanguageModel is the provider-agnostic single-call interface.
// Implementations must be safe to call concurrently.
type LanguageModel interface {
	// Stream returns a channel of events closed when the response ends or ctx is cancelled.
	Stream(ctx context.Context, p Params) (<-chan StreamEvent, error)
	// ModelID returns the model identifier (e.g. "claude-sonnet-4-6").
	ModelID() string
}

// Provider creates LanguageModel instances by model ID.
type Provider interface {
	Name() string
	Model(id string) LanguageModel
}
