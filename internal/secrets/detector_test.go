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

func TestDetector_ClassifyKnownCredentialFormats(t *testing.T) {
	detector := NewDetector()
	classification := detector.Classify("src/config/env.go", "file", "AWS key AKIA1234567890ABCDEF")

	if classification.Level != LevelSecretDenied {
		t.Fatalf("expected denied level for credential format, got %s", classification.Level)
	}
	if classification.Class != ClassA {
		t.Fatalf("expected class A for credential format, got %s", classification.Class)
	}
}

func TestDetector_ClassifyHighEntropyTokenAsCandidate(t *testing.T) {
	detector := NewDetector()
	classification := detector.Classify("src/main.go", "tool_output", "generated token: zY8pQ1mN4vR7tX2cK9dL6sH3wB5qJ8nF")

	if classification.Level != LevelSecretCandidate {
		t.Fatalf("expected secret candidate for high entropy token, got %s", classification.Level)
	}
	if classification.Class != ClassB {
		t.Fatalf("expected class B for high entropy token, got %s", classification.Class)
	}
}

func TestDetector_ClassifyContextAwareRestrictedPath(t *testing.T) {
	detector := NewDetector()
	classification := detector.Classify(".env.production", "file", "APP_ENV=prod")

	if classification.Level != LevelRestricted {
		t.Fatalf("expected restricted level for .env path, got %s", classification.Level)
	}
	if classification.Class != ClassC {
		t.Fatalf("expected class C for restricted path, got %s", classification.Class)
	}
}

func TestDetector_ClassifyNormalContent(t *testing.T) {
	detector := NewDetector()
	classification := detector.Classify("src/app.go", "file", "fmt.Println(\"hello\")")

	if classification.Level != LevelNormal {
		t.Fatalf("expected normal level for safe content, got %s", classification.Level)
	}
	if classification.Class != ClassD {
		t.Fatalf("expected class D for safe content, got %s", classification.Class)
	}
}
