package validation

import (
	"fmt"
	"sort"
	"strings"
)

type CompatibilityReport struct {
	BreakingChanges []string
	AdditiveChanges []string
}

func CompareSchemas(previous, candidate SchemaDefinition) CompatibilityReport {
	report := CompatibilityReport{
		BreakingChanges: make([]string, 0),
		AdditiveChanges: make([]string, 0),
	}

	for fieldName, previousField := range previous.Object {
		candidateField, exists := candidate.Object[fieldName]
		if !exists {
			report.BreakingChanges = append(report.BreakingChanges, fmt.Sprintf("removed field %s", fieldName))
			continue
		}
		if candidateField.Type != previousField.Type {
			report.BreakingChanges = append(report.BreakingChanges, fmt.Sprintf("changed type for field %s", fieldName))
		}
		if !previousField.Required && candidateField.Required {
			report.BreakingChanges = append(report.BreakingChanges, fmt.Sprintf("field %s became required", fieldName))
		}
	}

	for fieldName, candidateField := range candidate.Object {
		if _, exists := previous.Object[fieldName]; exists {
			continue
		}
		if candidateField.Required {
			report.BreakingChanges = append(report.BreakingChanges, fmt.Sprintf("added required field %s", fieldName))
			continue
		}
		report.AdditiveChanges = append(report.AdditiveChanges, fmt.Sprintf("added optional field %s", fieldName))
	}

	sort.Strings(report.BreakingChanges)
	sort.Strings(report.AdditiveChanges)

	return report
}

func ValidateUpgrade(previous, candidate SchemaDefinition) error {
	if previous.ID != candidate.ID {
		return fmt.Errorf("schema ID mismatch: %s != %s", previous.ID, candidate.ID)
	}

	report := CompareSchemas(previous, candidate)
	if len(report.BreakingChanges) == 0 {
		return nil
	}

	if previous.Version == candidate.Version {
		return fmt.Errorf("breaking schema changes require version bump: %s", strings.Join(report.BreakingChanges, ", "))
	}

	return nil
}
