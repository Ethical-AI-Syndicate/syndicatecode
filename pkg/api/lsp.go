package api

type LSPRange struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

type LSPDiagnostic struct {
	Path     string   `json:"path"`
	Code     string   `json:"code,omitempty"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Range    LSPRange `json:"range"`
}

type LSPSymbol struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Path  string   `json:"path"`
	Range LSPRange `json:"range"`
}

type LSPPositionRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
}

type LSPHoverResponse struct {
	Contents string   `json:"contents"`
	Range    LSPRange `json:"range"`
}

type LSPLocation struct {
	Path  string   `json:"path"`
	Range LSPRange `json:"range"`
}

type LSPCompletionItem struct {
	Label         string `json:"label"`
	Kind          string `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}
