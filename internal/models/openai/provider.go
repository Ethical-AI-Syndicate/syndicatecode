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

		// Fix I2: track whether MessageStartEvent has been emitted.
		startEmitted := false

		// Fix C3: map from tool call Index to its ID for correlation across chunks.
		toolIDs := make(map[int64]string)

		// Fix N1: accumulate FinishReason across chunks; the final usage chunk
		// arrives with choices:[] so we must save it from an earlier chunk.
		var stopReason string

		for stream.Next() {
			chunk := stream.Current()

			// Emit input tokens if present (typically on the first usage chunk).
			// Fix I2: only emit once.
			if chunk.Usage.PromptTokens != 0 && !startEmitted {
				startEmitted = true
				select {
				case ch <- models.MessageStartEvent{InputTokens: int(chunk.Usage.PromptTokens)}:
				case <-ctx.Done():
					return
				}
			}

			// Fix N1: save FinishReason whenever a choice carries it.
			if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				stopReason = chunk.Choices[0].FinishReason
			}

			// Emit output tokens and stop reason if present
			if chunk.Usage.CompletionTokens != 0 {
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
					// Fix C3: populate the ID map when the first chunk for a call arrives.
					if toolCall.ID != "" {
						toolIDs[toolCall.Index] = toolCall.ID
					}
					// Resolve the ID from the map (handles subsequent chunks with empty ID).
					resolvedID := toolIDs[toolCall.Index]

					// Tool call start with ID and name
					if toolCall.ID != "" && toolCall.Function.Name != "" {
						select {
						case ch <- models.ToolUseStartEvent{ID: resolvedID, Name: toolCall.Function.Name}:
						case <-ctx.Done():
							return
						}
					}

					// Tool input delta
					if toolCall.Function.Arguments != "" {
						select {
						case ch <- models.ToolInputDeltaEvent{ID: resolvedID, Delta: toolCall.Function.Arguments}:
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
	// Fix C1: prepend system message if present.
	var messages []openaisdk.ChatCompletionMessageParamUnion
	if p.System != "" {
		messages = append(messages, openaisdk.SystemMessage(p.System))
	}
	messages = append(messages, buildMessages(p.Messages)...)

	params := openaisdk.ChatCompletionNewParams{
		Model:    modelID,
		Messages: messages,
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
		switch msg.Role {
		case "assistant":
			out = append(out, buildAssistantMessage(msg.Content)...)
		default: // "user"
			out = append(out, buildUserMessages(msg.Content)...)
		}
	}
	return out
}

// buildAssistantMessage converts assistant content blocks to OpenAI message params.
// Fix C2: handle ToolUseBlock by building assistant messages with ToolCalls.
func buildAssistantMessage(blocks []models.ContentBlock) []openaisdk.ChatCompletionMessageParamUnion {
	var out []openaisdk.ChatCompletionMessageParamUnion

	// Collect tool calls from ToolUseBlocks.
	var toolCalls []openaisdk.ChatCompletionMessageToolCallUnionParam
	var text string
	for _, block := range blocks {
		switch b := block.(type) {
		case models.TextBlock:
			text = b.Text
		case models.ToolUseBlock:
			toolCalls = append(toolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
					ID: b.ID,
					Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      b.Name,
						Arguments: string(b.Input),
					},
				},
			})
		}
	}

	if len(toolCalls) > 0 {
		// Emit an assistant message with ToolCalls (and optionally text content).
		asst := &openaisdk.ChatCompletionAssistantMessageParam{
			ToolCalls: toolCalls,
		}
		if text != "" {
			asst.Content = openaisdk.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openaisdk.String(text),
			}
		}
		out = append(out, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: asst})
	} else {
		out = append(out, openaisdk.AssistantMessage(text))
	}

	return out
}

// buildUserMessages converts user content blocks to OpenAI message params.
// Fix C2: handle ToolResultBlock by mapping to ToolMessage.
func buildUserMessages(blocks []models.ContentBlock) []openaisdk.ChatCompletionMessageParamUnion {
	var out []openaisdk.ChatCompletionMessageParamUnion

	for _, block := range blocks {
		switch b := block.(type) {
		case models.TextBlock:
			out = append(out, openaisdk.UserMessage(b.Text))
		case models.ToolResultBlock:
			// Fix C2: map tool results to tool messages.
			out = append(out, openaisdk.ToolMessage(b.Content, b.ToolUseID))
		}
	}

	// If no blocks produced output, emit an empty user message.
	if len(out) == 0 {
		out = append(out, openaisdk.UserMessage(""))
	}

	return out
}
