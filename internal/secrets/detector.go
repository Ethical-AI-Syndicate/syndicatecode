package secrets

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

type ClassificationLevel string

const (
	LevelNormal          ClassificationLevel = "normal"
	LevelRestricted      ClassificationLevel = "restricted"
	LevelSecretCandidate ClassificationLevel = "secret_candidate"
	LevelSecretDenied    ClassificationLevel = "secret_denied"
)

type SensitivityClass string

const (
	ClassA SensitivityClass = "A"
	ClassB SensitivityClass = "B"
	ClassC SensitivityClass = "C"
	ClassD SensitivityClass = "D"
)

type Classification struct {
	Level   ClassificationLevel `json:"level"`
	Class   SensitivityClass    `json:"class"`
	Signals []string            `json:"signals"`
}

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

type RedactionReport struct {
	Applied        bool
	MaterialImpact bool
	MatchCount     int
	Reasons        []string
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

func (d *Detector) Classify(path, sourceType, content string) Classification {
	matches := d.Scan(content)
	signals := make([]string, 0)
	for _, match := range matches {
		signals = append(signals, match.RuleName)
	}

	if hasSignal(signals, "private_key") || hasSignal(signals, "aws_access_key") || hasSignal(signals, "github_pat") {
		return Classification{Level: LevelSecretDenied, Class: ClassA, Signals: signals}
	}

	if hasSignal(signals, "env_secret") || containsHighEntropyToken(content) {
		if containsHighEntropyToken(content) {
			signals = append(signals, "high_entropy")
		}
		return Classification{Level: LevelSecretCandidate, Class: ClassB, Signals: uniqueSignals(signals)}
	}

	if isRestrictedPath(path) || sourceType == "env" {
		signals = append(signals, "restricted_path")
		return Classification{Level: LevelRestricted, Class: ClassC, Signals: uniqueSignals(signals)}
	}

	return Classification{Level: LevelNormal, Class: ClassD, Signals: uniqueSignals(signals)}
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

func (d *Detector) RedactMapWithReport(input map[string]interface{}) (map[string]interface{}, RedactionReport) {
	reasons := map[string]struct{}{}
	matchCount := 0

	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case string:
			matches := d.Scan(typed)
			for _, match := range matches {
				reasons[match.RuleName] = struct{}{}
			}
			matchCount += len(matches)
			output[key] = d.RedactString(typed)
		case map[string]interface{}:
			nestedOutput, report := d.RedactMapWithReport(typed)
			output[key] = nestedOutput
			matchCount += report.MatchCount
			for _, reason := range report.Reasons {
				reasons[reason] = struct{}{}
			}
		case []interface{}:
			nestedOutput, report := d.redactSliceWithReport(typed)
			output[key] = nestedOutput
			matchCount += report.MatchCount
			for _, reason := range report.Reasons {
				reasons[reason] = struct{}{}
			}
		default:
			output[key] = value
		}
	}

	reasonList := make([]string, 0, len(reasons))
	for reason := range reasons {
		reasonList = append(reasonList, reason)
	}
	sort.Strings(reasonList)

	report := RedactionReport{
		Applied:        matchCount > 0,
		MaterialImpact: matchCount > 0,
		MatchCount:     matchCount,
		Reasons:        reasonList,
	}

	return output, report
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

func (d *Detector) redactSliceWithReport(input []interface{}) ([]interface{}, RedactionReport) {
	reasons := map[string]struct{}{}
	matchCount := 0

	output := make([]interface{}, 0, len(input))
	for _, item := range input {
		switch typed := item.(type) {
		case string:
			matches := d.Scan(typed)
			for _, match := range matches {
				reasons[match.RuleName] = struct{}{}
			}
			matchCount += len(matches)
			output = append(output, d.RedactString(typed))
		case map[string]interface{}:
			nestedOutput, report := d.RedactMapWithReport(typed)
			output = append(output, nestedOutput)
			matchCount += report.MatchCount
			for _, reason := range report.Reasons {
				reasons[reason] = struct{}{}
			}
		case []interface{}:
			nestedOutput, report := d.redactSliceWithReport(typed)
			output = append(output, nestedOutput)
			matchCount += report.MatchCount
			for _, reason := range report.Reasons {
				reasons[reason] = struct{}{}
			}
		default:
			output = append(output, item)
		}
	}

	reasonList := make([]string, 0, len(reasons))
	for reason := range reasons {
		reasonList = append(reasonList, reason)
	}
	sort.Strings(reasonList)

	report := RedactionReport{
		Applied:        matchCount > 0,
		MaterialImpact: matchCount > 0,
		MatchCount:     matchCount,
		Reasons:        reasonList,
	}

	return output, report
}

func RedactString(content string) string {
	return NewDetector().RedactString(content)
}

func hasSignal(signals []string, target string) bool {
	for _, signal := range signals {
		if signal == target {
			return true
		}
	}
	return false
}

func isRestrictedPath(path string) bool {
	lowerPath := strings.ToLower(path)
	return strings.Contains(lowerPath, ".env") || strings.Contains(lowerPath, "secret") || strings.Contains(lowerPath, "credential")
}

func containsHighEntropyToken(content string) bool {
	for _, token := range strings.Fields(content) {
		cleaned := strings.Trim(token, "\"'.,:;()[]{}")
		if len(cleaned) < 24 {
			continue
		}
		if shannonEntropy(cleaned) >= 3.8 {
			return true
		}
	}
	return false
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]float64{}
	for _, ch := range value {
		counts[ch]++
	}
	length := float64(len(value))
	entropy := 0.0
	for _, count := range counts {
		p := count / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func uniqueSignals(signals []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(signals))
	for _, signal := range signals {
		if _, exists := seen[signal]; exists {
			continue
		}
		seen[signal] = struct{}{}
		result = append(result, signal)
	}
	return result
}
