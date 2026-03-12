package audit

import "sort"

var knownEventTypes = map[string]struct{}{
	EventSessionStarted:          {},
	EventSessionTerminated:       {},
	EventTurnStarted:             {},
	EventTurnAwaitingApproval:    {},
	EventTurnCompleted:           {},
	EventTurnFailed:              {},
	EventTurnCancelled:           {},
	EventApprovalProposed:        {},
	EventApprovalDecided:         {},
	EventApprovalExecuted:        {},
	EventApprovalTransition:      {},
	EventMCPCall:                 {},
	EventToolRedaction:           {},
	EventRetentionClean:          {},
	EventContextFragment:         {},
	EventContextManifestEntry:    {},
	EventContextManifestConflict: {},
	EventModelInvoked:            {},
	EventToolInvoked:             {},
	EventToolResult:              {},
	EventFileMutation:            {},
	EventLSPRequest:              {},
}

func IsKnownEventType(eventType string) bool {
	_, ok := knownEventTypes[eventType]
	return ok
}

func KnownEventTypes() []string {
	types := make([]string, 0, len(knownEventTypes))
	for eventType := range knownEventTypes {
		types = append(types, eventType)
	}
	sort.Strings(types)
	return types
}
