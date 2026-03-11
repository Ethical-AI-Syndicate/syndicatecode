package openai

import (
	"context"
	"encoding/json"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

// Provider implements models.Provider for OpenAI.
type Provider struct {
	apiKey string
}

// NewProvider creates a Provider that authenticates with the given API key.
func NewProvider(apiKey string) *Provider {
	return &Provider{apiKey: apiKey}
}

// Name returns the canonical provider name.
func (p *Provider) Name() string { return "openai" }

// Model returns a LanguageModel backed by the given model ID.
func (p *Provider) Model(id string) models.LanguageModel {
	client := openaisdk.NewClient(option.WithAPIKey(p.apiKey))
	return &model{client: &client, id: id}
}

// model implements models.LanguageModel using the OpenAI Chat Completions API.
type model struct {
	client *openaisdk.Client
	id     string
}

// ModelID returns the model identifier (e.g. "gpt-4o").
func (m *model) ModelID() string { return m.id }

// Stream opens a streaming Chat Completions request and publishes events to the
// returned channel. The channel is closed when the response ends, ctx is
// cancelled, or an error is encountered.
func (m *model) Stream(ctx context.Context, p models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent, 32)
	go func() {
		defer close(ch)

		// Build SDK chat completion params.
		params := buildChatCompletionParams(m.id, p)

		stream := m.client.Chat.Completions.NewStreaming(ctx, params)
		defer func() { _ = stream.Close() }()

		for stream.Next() {
			chunk := stream.Current()

			// Emit input tokens if present (typically on the final usage chunk)
			if chunk.Usage.PromptTokens != 0 {
				select {
				case ch <- models.MessageStartEvent{InputTokens: int(chunk.Usage.PromptTokens)}:
				case <-ctx.Done():
					return
				}
			}

			// Emit output tokens and stop reason if present
			if chunk.Usage.CompletionTokens != 0 {
				var stopReason string
				if len(chunk.Choices) > 0 {
					stopReason = chunk.Choices[0].FinishReason
				}
				select {
				case ch <- models.MessageDeltaEvent{
					OutputTokens: int(chunk.Usage.CompletionTokens),
					StopReason:   stopReason,
				}:
				case <-ctx.Done():
					return
				}
			}

			// Process each choice
			for _, choice := range chunk.Choices {
				delta := choice.Delta

				// Text content
				if delta.Content != "" {
					select {
					case ch <- models.TextDeltaEvent{Delta: delta.Content}:
					case <-ctx.Done():
						return
					}
				}

				// Tool calls
				for _, toolCall := range delta.ToolCalls {
					// Tool call start with ID and name
					if toolCall.ID != "" && toolCall.Function.Name != "" {
						select {
						case ch <- models.ToolUseStartEvent{ID: toolCall.ID, Name: toolCall.Function.Name}:
						case <-ctx.Done():
							return
						}
					}

					// Tool input delta
					if toolCall.Function.Arguments != "" {
						select {
						case ch <- models.ToolInputDeltaEvent{ID: toolCall.ID, Delta: toolCall.Function.Arguments}:
						case <-ctx.Done():
							return
						}
					}
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

// buildChatCompletionParams converts our agnostic Params to the SDK's ChatCompletionNewParams.
func buildChatCompletionParams(modelID string, p models.Params) openaisdk.ChatCompletionNewParams {
	params := openaisdk.ChatCompletionNewParams{
		Model:    modelID,
		Messages: buildMessages(p.Messages),
		StreamOptions: openaisdk.ChatCompletionStreamOptionsParam{
			IncludeUsage: openaisdk.Bool(true),
		},
	}

	// MaxTokens
	if p.MaxTokens > 0 {
		params.MaxTokens = openaisdk.Int(int64(p.MaxTokens))
	}

	// Tools
	for _, t := range p.Tools {
		var funcParams shared.FunctionParameters
		if err := json.Unmarshal(t.InputSchema, &funcParams); err != nil {
			// Fall back: use empty parameters
			funcParams = shared.FunctionParameters{}
		}

		funcDef := shared.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: funcParams,
		}
		if t.Description != "" {
			funcDef.Description = openaisdk.String(t.Description)
		}

		toolParam := openaisdk.ChatCompletionFunctionTool(funcDef)
		params.Tools = append(params.Tools, toolParam)
	}

	return params
}

// buildMessages converts our agnostic Message slice to OpenAI ChatCompletionMessageParamUnion slice.
func buildMessages(messages []models.Message) []openaisdk.ChatCompletionMessageParamUnion {
	out := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		// Extract text content from first TextBlock if present
		var text string
		for _, block := range msg.Content {
			if tb, ok := block.(models.TextBlock); ok {
				text = tb.Text
				break
			}
		}

		switch msg.Role {
		case "assistant":
			out = append(out, openaisdk.AssistantMessage(text))
		default: // "user"
			out = append(out, openaisdk.UserMessage(text))
		}
	}
	return out
}
