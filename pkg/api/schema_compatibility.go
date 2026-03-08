package api

import (
	"errors"
	"fmt"
)

var ErrInvalidMigrationPlan = errors.New("invalid migration plan")

type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type SchemaDefinition struct {
	Name    string        `json:"name"`
	Version int           `json:"version"`
	Fields  []SchemaField `json:"fields"`
}

type CompatibilityReport struct {
	Compatible bool
	Issues     []string
}

type MigrationPlan struct {
	MigrationSteps        []string
	DeprecationWindowDays int
}

func CheckCompatibility(previous, next SchemaDefinition) CompatibilityReport {
	issues := make([]string, 0)

	prevFields := make(map[string]SchemaField)
	for _, field := range previous.Fields {
		prevFields[field.Name] = field
	}

	nextFields := make(map[string]SchemaField)
	for _, field := range next.Fields {
		nextFields[field.Name] = field
	}

	hasBreaking := false
	for name, prevField := range prevFields {
		nextField, exists := nextFields[name]
		if !exists {
			hasBreaking = true
			issues = append(issues, fmt.Sprintf("field removed: %s", name))
			continue
		}
		if prevField.Type != nextField.Type {
			hasBreaking = true
			issues = append(issues, fmt.Sprintf("field type changed for %s: %s -> %s", name, prevField.Type, nextField.Type))
		}
		if prevField.Required && !nextField.Required {
			issues = append(issues, fmt.Sprintf("field relaxed from required to optional: %s", name))
		}
	}

	if hasBreaking && next.Version <= previous.Version {
		issues = append(issues, "breaking changes require schema version bump")
		return CompatibilityReport{Compatible: false, Issues: issues}
	}

	return CompatibilityReport{Compatible: true, Issues: issues}
}

func ValidateMigrationPolicy(previous, next SchemaDefinition, plan MigrationPlan) error {
	report := CheckCompatibility(previous, next)
	hasBreaking := !report.Compatible

	if hasBreaking && len(plan.MigrationSteps) == 0 {
		return fmt.Errorf("%w: breaking changes require explicit migration steps", ErrInvalidMigrationPlan)
	}
	if !hasBreaking && plan.DeprecationWindowDays <= 0 {
		return fmt.Errorf("%w: non-breaking schema changes require deprecation window", ErrInvalidMigrationPlan)
	}

	return nil
}
