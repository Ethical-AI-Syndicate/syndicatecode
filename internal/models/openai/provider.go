package openai

import (
	"context"
	"encoding/json"
	"strings"

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

type toolCallDeltaState struct {
	ID      string
	Name    string
	Started bool
}

func (m *model) ProviderName() string { return "openai" }

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

		toolCallStatesByIndex := map[int64]*toolCallDeltaState{}

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

				// Tool calls.
				for _, event := range collectToolCallDeltaEvents(delta.ToolCalls, toolCallStatesByIndex) {
					select {
					case ch <- event:
					case <-ctx.Done():
						return
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

func collectToolCallDeltaEvents(toolCalls []openaisdk.ChatCompletionChunkChoiceDeltaToolCall, statesByIndex map[int64]*toolCallDeltaState) []models.StreamEvent {
	events := make([]models.StreamEvent, 0, len(toolCalls)*2)

	for _, toolCall := range toolCalls {
		state := statesByIndex[toolCall.Index]
		if state == nil {
			state = &toolCallDeltaState{}
			statesByIndex[toolCall.Index] = state
		}

		if toolCall.ID != "" {
			state.ID = toolCall.ID
		}
		if toolCall.Function.Name != "" {
			state.Name = toolCall.Function.Name
		}

		resolvedID := toolCall.ID
		if resolvedID == "" {
			resolvedID = state.ID
		}

		if !state.Started && resolvedID != "" && state.Name != "" {
			events = append(events, models.ToolUseStartEvent{ID: resolvedID, Name: state.Name})
			state.Started = true
		}

		if toolCall.Function.Arguments != "" {
			events = append(events, models.ToolInputDeltaEvent{ID: resolvedID, Delta: toolCall.Function.Arguments})
		}
	}

	return events
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
		switch msg.Role {
		case "assistant":
			textParts := make([]string, 0, len(msg.Content))
			toolCalls := make([]openaisdk.ChatCompletionMessageToolCallUnionParam, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch v := block.(type) {
				case models.TextBlock:
					textParts = append(textParts, v.Text)
				case models.ToolUseBlock:
					toolCalls = append(toolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
							ID: v.ID,
							Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      v.Name,
								Arguments: string(v.Input),
							},
						},
					})
				}
			}
			if len(textParts) == 0 && len(toolCalls) == 0 {
				continue
			}

			assistantMsg := openaisdk.ChatCompletionAssistantMessageParam{}
			if len(textParts) > 0 {
				assistantMsg = *openaisdk.AssistantMessage(strings.Join(textParts, "\n")).OfAssistant
			}
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, toolCalls...)
			out = append(out, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg})
		default: // "user"
			textParts := make([]string, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch v := block.(type) {
				case models.TextBlock:
					textParts = append(textParts, v.Text)
				case models.ToolResultBlock:
					out = append(out, openaisdk.ToolMessage(v.Content, v.ToolUseID))
				}
			}
			if len(textParts) > 0 {
				out = append(out, openaisdk.UserMessage(strings.Join(textParts, "\n")))
			}
		}
	}
	return out
}
