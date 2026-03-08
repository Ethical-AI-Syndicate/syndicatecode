package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Destination string

const (
	DestinationModelProvider Destination = "model_provider"
	DestinationPersistence   Destination = "persistence"
	DestinationExport        Destination = "export"
	DestinationPlugin        Destination = "plugin"
	DestinationUI            Destination = "ui"
)

type RedactionAction string

const (
	ActionAllow       RedactionAction = "allow"
	ActionDeny        RedactionAction = "deny"
	ActionMask        RedactionAction = "mask"
	ActionPartialMask RedactionAction = "partial-mask"
	ActionSummarize   RedactionAction = "summarize"
	ActionHash        RedactionAction = "hash"
)

type PolicyDecision struct {
	Destination    Destination     `json:"destination"`
	Action         RedactionAction `json:"action"`
	Classification Classification  `json:"classification"`
	Content        string          `json:"content"`
	Denied         bool            `json:"denied"`
	Reason         string          `json:"reason"`
}

type PolicyExecutor struct {
	detector *Detector
}

func NewPolicyExecutor(detector *Detector) *PolicyExecutor {
	if detector == nil {
		detector = NewDetector()
	}
	return &PolicyExecutor{detector: detector}
}

func (e *PolicyExecutor) Apply(path, sourceType, content string, destination Destination) PolicyDecision {
	classification := e.detector.Classify(path, sourceType, content)
	action := e.actionFor(classification.Class, destination)
	transformed, denied := e.transform(action, classification, content)

	return PolicyDecision{
		Destination:    destination,
		Action:         action,
		Classification: classification,
		Content:        transformed,
		Denied:         denied,
		Reason:         fmt.Sprintf("%s policy for %s destination", classification.Class, destination),
	}
}

func (e *PolicyExecutor) ApplyMap(path, sourceType string, destination Destination, input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}

	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		fieldPath := path + "." + key
		switch typed := value.(type) {
		case string:
			decision := e.Apply(fieldPath, sourceType, typed, destination)
			output[key] = decision.Content
		case map[string]interface{}:
			output[key] = e.ApplyMap(fieldPath, sourceType, destination, typed)
		case []interface{}:
			output[key] = e.applySlice(fieldPath, sourceType, destination, typed)
		default:
			output[key] = value
		}
	}

	return output
}

func (e *PolicyExecutor) applySlice(path, sourceType string, destination Destination, input []interface{}) []interface{} {
	output := make([]interface{}, 0, len(input))
	for idx, item := range input {
		fieldPath := fmt.Sprintf("%s[%d]", path, idx)
		switch typed := item.(type) {
		case string:
			decision := e.Apply(fieldPath, sourceType, typed, destination)
			output = append(output, decision.Content)
		case map[string]interface{}:
			output = append(output, e.ApplyMap(fieldPath, sourceType, destination, typed))
		case []interface{}:
			output = append(output, e.applySlice(fieldPath, sourceType, destination, typed))
		default:
			output = append(output, item)
		}
	}
	return output
}

func (e *PolicyExecutor) actionFor(class SensitivityClass, destination Destination) RedactionAction {
	switch class {
	case ClassA:
		switch destination {
		case DestinationPersistence:
			return ActionHash
		case DestinationUI:
			return ActionSummarize
		default:
			return ActionDeny
		}
	case ClassB:
		switch destination {
		case DestinationModelProvider:
			return ActionMask
		case DestinationPersistence, DestinationExport:
			return ActionHash
		case DestinationPlugin:
			return ActionDeny
		case DestinationUI:
			return ActionPartialMask
		default:
			return ActionMask
		}
	case ClassC:
		switch destination {
		case DestinationModelProvider:
			return ActionSummarize
		case DestinationExport:
			return ActionMask
		case DestinationPlugin:
			return ActionPartialMask
		default:
			return ActionAllow
		}
	default:
		return ActionAllow
	}
}

func (e *PolicyExecutor) transform(action RedactionAction, classification Classification, content string) (string, bool) {
	switch action {
	case ActionDeny:
		return "[DENIED]", true
	case ActionMask:
		redacted := e.detector.RedactString(content)
		if redacted == content {
			return "[REDACTED]", false
		}
		return redacted, false
	case ActionPartialMask:
		return partialMask(content), false
	case ActionSummarize:
		return fmt.Sprintf("[REDACTED summary: %s content withheld]", classification.Class), false
	case ActionHash:
		hash := sha256.Sum256([]byte(content))
		return "sha256:" + hex.EncodeToString(hash[:]), false
	default:
		return content, false
	}
}

func partialMask(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "[REDACTED]"
	}
	if len(trimmed) <= 8 {
		return "[REDACTED]"
	}
	return trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
}
