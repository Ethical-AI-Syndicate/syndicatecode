package anthropic

import (
	"context"
	"encoding/json"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

// Provider implements models.Provider for Anthropic.
type Provider struct {
	apiKey string
}

// NewProvider creates a Provider that authenticates with the given API key.
func NewProvider(apiKey string) *Provider {
	return &Provider{apiKey: apiKey}
}

// Name returns the canonical provider name.
func (p *Provider) Name() string { return "anthropic" }

// Model returns a LanguageModel backed by the given model ID.
func (p *Provider) Model(id string) models.LanguageModel {
	client := anthropicsdk.NewClient(option.WithAPIKey(p.apiKey))
	return &model{client: client, id: id}
}

// model implements models.LanguageModel using the Anthropic Messages API.
type model struct {
	client anthropicsdk.Client
	id     string
}

func (m *model) ProviderName() string { return "anthropic" }

// ModelID returns the model identifier (e.g. "claude-sonnet-4-6").
func (m *model) ModelID() string { return m.id }

// Stream opens a streaming Messages request and publishes events to the
// returned channel. The channel is closed when the response ends, ctx is
// cancelled, or an error is encountered.
func (m *model) Stream(ctx context.Context, p models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent, 32)
	go func() {
		defer close(ch)

		toolUseIDByBlockIndex := map[int64]string{}

		// Build SDK message params.
		msgParams := buildMessageParams(p)

		stream := m.client.Messages.NewStreaming(ctx, msgParams)
		defer func() { _ = stream.Close() }()

		for stream.Next() {
			evt := stream.Current()
			switch evt.Type {
			case "message_start":
				ms := evt.AsMessageStart()
				select {
				case ch <- models.MessageStartEvent{InputTokens: int(ms.Message.Usage.InputTokens)}:
				case <-ctx.Done():
					return
				}
			case "content_block_start":
				cb := evt.AsContentBlockStart()
				if cb.ContentBlock.Type == "tool_use" {
					tu := cb.ContentBlock.AsToolUse()
					toolUseIDByBlockIndex[cb.Index] = tu.ID
					select {
					case ch <- models.ToolUseStartEvent{ID: tu.ID, Name: tu.Name}:
					case <-ctx.Done():
						return
					}
				}
			case "content_block_delta":
				cbd := evt.AsContentBlockDelta()
				delta := cbd.Delta
				switch delta.Type {
				case "text_delta":
					td := delta.AsTextDelta()
					select {
					case ch <- models.TextDeltaEvent{Delta: td.Text}:
					case <-ctx.Done():
						return
					}
				case "input_json_delta":
					ijd := delta.AsInputJSONDelta()
					inputEvt := buildToolInputDeltaEvent(cbd.Index, ijd.PartialJSON, toolUseIDByBlockIndex)
					select {
					case ch <- inputEvt:
					case <-ctx.Done():
						return
					}
				}
			case "message_delta":
				md := evt.AsMessageDelta()
				select {
				case ch <- models.MessageDeltaEvent{
					OutputTokens: int(md.Usage.OutputTokens),
					StopReason:   string(md.Delta.StopReason),
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case ch <- models.MessageDeltaEvent{StopReason: "error"}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func buildToolInputDeltaEvent(blockIndex int64, partialJSON string, toolUseIDByBlockIndex map[int64]string) models.ToolInputDeltaEvent {
	return models.ToolInputDeltaEvent{ID: toolUseIDByBlockIndex[blockIndex], Delta: partialJSON}
}

// buildMessageParams converts our agnostic Params to the SDK's MessageNewParams.
func buildMessageParams(p models.Params) anthropicsdk.MessageNewParams {
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(p.Model),
		MaxTokens: int64(maxTokens),
	}

	// System prompt.
	if p.System != "" {
		params.System = []anthropicsdk.TextBlockParam{
			{Text: p.System},
		}
	}

	// Messages.
	for _, msg := range p.Messages {
		blocks := convertContentBlocks(msg.Content)
		switch msg.Role {
		case "assistant":
			params.Messages = append(params.Messages, anthropicsdk.NewAssistantMessage(blocks...))
		default: // "user"
			params.Messages = append(params.Messages, anthropicsdk.NewUserMessage(blocks...))
		}
	}

	// Tools.
	for _, t := range p.Tools {
		var schema anthropicsdk.ToolInputSchemaParam
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			// Fall back: leave schema as zero value (object type only).
			schema = anthropicsdk.ToolInputSchemaParam{}
		}
		params.Tools = append(params.Tools, anthropicsdk.ToolUnionParamOfTool(schema, t.Name))
		// Set description on the last tool entry if present.
		if t.Description != "" && params.Tools[len(params.Tools)-1].OfTool != nil {
			params.Tools[len(params.Tools)-1].OfTool.Description = anthropicsdk.String(t.Description)
		}
	}

	return params
}

// convertContentBlocks maps our ContentBlock sum type to SDK ContentBlockParamUnion values.
func convertContentBlocks(blocks []models.ContentBlock) []anthropicsdk.ContentBlockParamUnion {
	out := make([]anthropicsdk.ContentBlockParamUnion, 0, len(blocks))
	for _, b := range blocks {
		switch v := b.(type) {
		case models.TextBlock:
			out = append(out, anthropicsdk.NewTextBlock(v.Text))
		case models.ToolUseBlock:
			var input any
			if err := json.Unmarshal(v.Input, &input); err != nil {
				input = map[string]any{}
			}
			out = append(out, anthropicsdk.NewToolUseBlock(v.ID, input, v.Name))
		case models.ToolResultBlock:
			out = append(out, anthropicsdk.NewToolResultBlock(v.ToolUseID, v.Content, v.IsError))
		}
	}
	return out
}
