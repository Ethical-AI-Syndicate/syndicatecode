package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/validation"
)

type schemaValueType string

const (
	schemaTypeString  schemaValueType = "string"
	schemaTypeBoolean schemaValueType = "boolean"
	schemaTypeNumber  schemaValueType = "number"
	schemaTypeObject  schemaValueType = "object"
	schemaTypeArray   schemaValueType = "array"
)

type schemaField struct {
	Type     schemaValueType
	Required bool
}

type jsonObjectSchema map[string]schemaField

type schemaViolation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type schemaErrorPayload struct {
	Error      string            `json:"error"`
	Violations []schemaViolation `json:"violations"`
}

type responseSchemaByStatus map[string]map[int]jsonObjectSchema

func schemaValidationMiddleware(requestSchemas, responseSchemas map[string]jsonObjectSchema, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqSchema, ok := requestSchemas[r.Method]; ok {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeSchemaError(w, http.StatusBadRequest, []schemaViolation{{Field: "body", Message: fmt.Sprintf("failed to read request body: %v", err)}})
				return
			}
			violations := validateJSONObject(body, reqSchema)
			if len(violations) > 0 {
				writeSchemaError(w, http.StatusBadRequest, violations)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		respSchema, validateResponse := responseSchemas[r.Method]
		if !validateResponse {
			next.ServeHTTP(w, r)
			return
		}

		capture := newCaptureResponseWriter()
		next.ServeHTTP(capture, r)

		statusCode := capture.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if statusCode >= http.StatusBadRequest {
			writeCapturedResponse(w, capture, statusCode)
			return
		}

		violations := validateJSONObject(capture.body.Bytes(), respSchema)
		if len(violations) > 0 {
			writeSchemaError(w, http.StatusInternalServerError, violations)
			return
		}

		writeCapturedResponse(w, capture, statusCode)
	})
}

func sessionsCreateRequestSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPISessionCreateRequest)
}

func sessionsCreateResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPISessionCreateResponse)
}

func turnsCreateRequestSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPITurnCreateRequest)
}

func turnsCreateResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPITurnCreateResponse)
}

func sessionTurnsCreateRequestSchema() jsonObjectSchema {
	return turnsCreateRequestSchema()
}

func toolsExecuteRequestSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPIToolExecuteRequest)
}

func toolsExecuteResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPIToolExecuteResponse)
}

func healthResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func readinessResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func metricsResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func approvalsDecisionRequestSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func approvalsDecisionResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func policyResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func toolsListResponseSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func approvalRecordSchema() jsonObjectSchema {
	return jsonObjectSchema{}
}

func lspPositionRequestSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPPositionRequest)
}

func lspDiagnosticsResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPDiagnosticsResult)
}

func lspSymbolsResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPSymbolsResult)
}

func lspHoverResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPHoverResult)
}

func lspDefinitionResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPDefinitionResult)
}

func lspReferencesResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPReferencesResult)
}

func lspCompletionsResponseSchema() jsonObjectSchema {
	return mustSchemaFromRegistry(validation.ContractAPILSPCompletionsResult)
}

func schemaValidationMiddlewareWithStatus(requestSchemas, responseSchemas map[string]jsonObjectSchema, responseSchemasByStatus responseSchemaByStatus, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqSchema, ok := requestSchemas[r.Method]; ok {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeSchemaError(w, http.StatusBadRequest, []schemaViolation{{Field: "body", Message: fmt.Sprintf("failed to read request body: %v", err)}})
				return
			}
			violations := validateJSONObject(body, reqSchema)
			if len(violations) > 0 {
				writeSchemaError(w, http.StatusBadRequest, violations)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		capture := newCaptureResponseWriter()
		next.ServeHTTP(capture, r)

		statusCode := capture.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if statusCode >= http.StatusBadRequest {
			writeCapturedResponse(w, capture, statusCode)
			return
		}

		respSchema, hasMethodSchema := responseSchemas[r.Method]
		if byStatus, ok := responseSchemasByStatus[r.Method]; ok {
			if statusSchema, ok := byStatus[statusCode]; ok {
				respSchema = statusSchema
				hasMethodSchema = true
			}
		}

		if hasMethodSchema {
			violations := validateJSONObject(capture.body.Bytes(), respSchema)
			if len(violations) > 0 {
				writeSchemaError(w, http.StatusInternalServerError, violations)
				return
			}
		}

		writeCapturedResponse(w, capture, statusCode)
	})
}

func mustSchemaFromRegistry(contractID string) jsonObjectSchema {
	definition, ok := validation.DefaultRegistry().Get(contractID)
	if !ok {
		panic(fmt.Sprintf("missing schema definition: %s", contractID))
	}

	result := make(jsonObjectSchema, len(definition.Object))
	for fieldName, fieldSpec := range definition.Object {
		result[fieldName] = schemaField{
			Type:     schemaValueType(fieldSpec.Type),
			Required: fieldSpec.Required,
		}
	}

	return result
}

func validateJSONObject(body []byte, schema jsonObjectSchema) []schemaViolation {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		body = []byte("{}")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return []schemaViolation{{Field: "body", Message: fmt.Sprintf("invalid JSON payload: %v", err)}}
	}

	violations := make([]schemaViolation, 0)
	for fieldName, fieldSchema := range schema {
		value, found := payload[fieldName]
		if !found {
			if fieldSchema.Required {
				violations = append(violations, schemaViolation{Field: fieldName, Message: "field is required"})
			}
			continue
		}

		if !matchesSchemaType(value, fieldSchema.Type) {
			violations = append(violations, schemaViolation{Field: fieldName, Message: fmt.Sprintf("expected type %s", fieldSchema.Type)})
		}
	}

	return violations
}

func matchesSchemaType(value interface{}, expectedType schemaValueType) bool {
	switch expectedType {
	case schemaTypeString:
		_, ok := value.(string)
		return ok
	case schemaTypeBoolean:
		_, ok := value.(bool)
		return ok
	case schemaTypeNumber:
		_, ok := value.(float64)
		return ok
	case schemaTypeObject:
		_, ok := value.(map[string]interface{})
		return ok
	case schemaTypeArray:
		_, ok := value.([]interface{})
		return ok
	default:
		return false
	}
}

func writeSchemaError(w http.ResponseWriter, statusCode int, violations []schemaViolation) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(schemaErrorPayload{Error: "invalid_schema", Violations: violations}); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode schema error: %v", err), http.StatusInternalServerError)
	}
}

type captureResponseWriter struct {
	headers    http.Header
	body       bytes.Buffer
	statusCode int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{headers: make(http.Header)}
}

func (w *captureResponseWriter) Header() http.Header {
	return w.headers
}

func (w *captureResponseWriter) Write(body []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *captureResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
}

func writeCapturedResponse(w http.ResponseWriter, capture *captureResponseWriter, statusCode int) {
	for key, values := range capture.headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(statusCode)
	if _, err := w.Write(capture.body.Bytes()); err != nil {
		return
	}
}
