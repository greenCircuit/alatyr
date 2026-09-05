package istio

import (
	"log/slog"
	"strconv"

	"alatyr/internal/models"
)

// convertPorts converts Istio Operation.Ports (string slice, e.g. "8080")
// into models.Port. Istio always implies TCP at the AuthZ layer.
// Returns nil for empty input — that means "all ports allowed."
//
// NotPorts is handled at the rule-expansion layer (subtraction); this helper
// is for the positive Ports list only.
//
// Unparseable entries are logged via slog.Default() (set by main.go). Policy
// ref isn't in scope here — keep the helper pure so tests don't churn.
func convertPorts(ports []string) []models.Port {
	if len(ports) == 0 {
		return nil
	}
	out := make([]models.Port, 0, len(ports))
	for _, raw := range ports {
		portNum, err := strconv.Atoi(raw)
		if err != nil {
			slog.Default().Warn("istio port parse failed",
				slog.String("phase", "istio_convert_ports"),
				slog.String("engine", sourceName),
				slog.String("raw", raw),
				slog.String("error", err.Error()),
			)
			continue
		}
		out = append(out, models.Port{Port: portNum, Protocol: "TCP"})
	}
	return out
}
