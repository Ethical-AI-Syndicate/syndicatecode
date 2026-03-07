package secrets

import (
	"regexp"
)

type Match struct {
	RuleName string
	Value    string
}

type rule struct {
	name  string
	regex *regexp.Regexp
}

type Detector struct {
	rules []rule
}

func NewDetector() *Detector {
	return &Detector{
		rules: []rule{
			{name: "aws_access_key", regex: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
			{name: "github_pat", regex: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`)},
			{name: "private_key", regex: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
			{name: "env_secret", regex: regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password)\s*=\s*[^\s]+`)},
		},
	}
}

func (d *Detector) Scan(content string) []Match {
	matches := make([]Match, 0)
	for _, r := range d.rules {
		found := r.regex.FindAllString(content, -1)
		for _, value := range found {
			matches = append(matches, Match{RuleName: r.name, Value: value})
		}
	}
	return matches
}

func (d *Detector) RedactString(content string) string {
	redacted := content
	for _, r := range d.rules {
		redacted = r.regex.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}

func (d *Detector) RedactMap(input map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{}, len(input))
	for k, v := range input {
		switch typed := v.(type) {
		case string:
			output[k] = d.RedactString(typed)
		case map[string]interface{}:
			output[k] = d.RedactMap(typed)
		case []interface{}:
			output[k] = d.redactSlice(typed)
		default:
			output[k] = v
		}
	}
	return output
}

func (d *Detector) redactSlice(input []interface{}) []interface{} {
	output := make([]interface{}, 0, len(input))
	for _, item := range input {
		switch typed := item.(type) {
		case string:
			output = append(output, d.RedactString(typed))
		case map[string]interface{}:
			output = append(output, d.RedactMap(typed))
		case []interface{}:
			output = append(output, d.redactSlice(typed))
		default:
			output = append(output, item)
		}
	}
	return output
}

func RedactString(content string) string {
	return NewDetector().RedactString(content)
}
