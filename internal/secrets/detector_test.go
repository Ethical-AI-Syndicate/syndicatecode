package secrets

import "testing"

func TestDetector_ScanFindsCommonSecrets(t *testing.T) {
	detector := NewDetector()
	content := "AWS key: AKIA1234567890ABCDEF\nprivate key: -----BEGIN PRIVATE KEY-----"

	matches := detector.Scan(content)
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 secret matches, got %d", len(matches))
	}
}

func TestDetector_RedactString(t *testing.T) {
	detector := NewDetector()
	content := "token=ghp_0123456789abcdefghijklmnopqrstuvwxyzAB"

	redacted := detector.RedactString(content)
	if redacted == content {
		t.Fatal("expected redaction to modify content")
	}
	if redacted == "" {
		t.Fatal("expected non-empty redacted content")
	}
}

func TestDetector_RedactMap(t *testing.T) {
	detector := NewDetector()
	input := map[string]interface{}{
		"safe": "hello",
		"nested": map[string]interface{}{
			"secret": "AKIA1234567890ABCDEF",
		},
	}

	output := detector.RedactMap(input)
	nested, ok := output["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("expected nested map")
	}
	if nested["secret"] == "AKIA1234567890ABCDEF" {
		t.Fatal("expected nested secret to be redacted")
	}
}
