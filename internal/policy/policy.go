package policy

import (
	"context"

	"graph/internal/models"
)

type PolicySource interface {
	// Stable identifier. Used as map key in Edge.Contributor and as
	// value in the API ?policySource= param.
	Name() string

	// Evaluate fetches policy data for the given namespaces and returns all
	// allow/deny rules and status keys. Fetch strategy is an internal detail.
	// Returns an error when policy data cannot be fetched — callers must
	// surface this rather than render an empty graph (silent data loss).
	Evaluate(ctx context.Context, namespaces []string, index map[string]models.NSIndex) (models.EvaluationResult, error)
}
