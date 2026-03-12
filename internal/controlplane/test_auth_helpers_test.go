package controlplane

import (
	"net/http"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/requestmeta"
)

func withOperatorRole(req *http.Request) *http.Request {
	ctx := requestmeta.WithRole(req.Context(), "operator")
	return req.WithContext(ctx)
}
