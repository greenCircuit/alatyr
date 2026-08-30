package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// Options controls one scan run. Format picks the stdout rendering; JSONPath,
// when set, additionally writes the machine-readable document to that file so
// CI gets a readable log and an artifact from a single invocation.
type Options struct {
	ManifestDir string
	Format      string // "table" (default) or "json"
	JSONPath    string
	Logger      *slog.Logger
}

// Run scans the manifest dir and renders the report. Logs stay on stderr so
// stdout only ever carries the chosen format.
func Run(options Options) error {
	out, err := makeReport(options.ManifestDir, options.Logger)
	if err != nil {
		return err
	}

	if options.JSONPath != "" {
		file, err := os.Create(options.JSONPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", options.JSONPath, err)
		}
		defer file.Close()
		if err := writeJSON(file, out); err != nil {
			return err
		}
	}

	switch options.Format {
	case "", "table":
		return renderTable(os.Stdout, out)
	case "json":
		return writeJSON(os.Stdout, out)
	default:
		return fmt.Errorf("unknown format %q (want table or json)", options.Format)
	}
}

func writeJSON(writer *os.File, out report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}
