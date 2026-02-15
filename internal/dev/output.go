package dev

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
)

// OutputFormat represents the output format
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

// OutputWriter formats and writes results to output
type OutputWriter interface {
	// WriteValidationResult writes validation results
	WriteValidationResult(result *ValidationResult) error
	// WriteTraceResult writes execution trace results
	WriteTraceResult(result *TraceResult) error
	// WriteError writes an error message
	WriteError(err error) error
}

// NewOutputWriter creates an OutputWriter for the given format
func NewOutputWriter(out io.Writer, format OutputFormat, verbose bool) OutputWriter {
	switch format {
	case OutputFormatJSON:
		return &jsonOutputWriter{out: out}
	default:
		return &textOutputWriter{out: out, verbose: verbose}
	}
}

// textOutputWriter writes human-readable text output
type textOutputWriter struct {
	out     io.Writer
	verbose bool
}

func (w *textOutputWriter) WriteValidationResult(result *ValidationResult) error {
	if result.Details != nil {
		w.writeCategory("Schema Validation", result.Details.Schema)
		w.writeCategory("Parameter Validation", result.Details.Params)
		w.writeCategory("CEL Expressions", result.Details.CEL)
		w.writeCategory("Go Templates", result.Details.Templates)
		w.writeCategory("K8s Manifests", result.Details.Manifests)
		_, _ = fmt.Fprintln(w.out)
	}

	if result.Valid {
		_, _ = fmt.Fprintln(w.out, "Validation: SUCCESS")
	} else {
		_, _ = fmt.Fprintln(w.out, "Validation: FAILED")
		_, _ = fmt.Fprintln(w.out)
		for _, err := range result.Errors {
			_, _ = fmt.Fprintf(w.out, "  [ERROR] %s: %s\n", err.Path, err.Message)
		}
	}

	if w.verbose && len(result.Warnings) > 0 {
		_, _ = fmt.Fprintln(w.out)
		for _, warn := range result.Warnings {
			_, _ = fmt.Fprintf(w.out, "  [WARN] %s: %s\n", warn.Path, warn.Message)
		}
	}

	return nil
}

func (w *textOutputWriter) writeCategory(name string, cat ValidationCategory) {
	status := "PASS"
	if !cat.Passed {
		status = "FAIL"
	}
	countStr := ""
	if cat.Count > 0 {
		countStr = fmt.Sprintf(" (%d items)", cat.Count)
	}
	_, _ = fmt.Fprintf(w.out, "%-25s %s%s\n", name+":", status, countStr)
}

func (w *textOutputWriter) WriteTraceResult(result *TraceResult) error {
	_, _ = fmt.Fprintf(w.out, "Dry-Run Execution Results\n")
	_, _ = fmt.Fprintf(w.out, "%s\n\n", strings.Repeat("=", 25))

	for _, phase := range result.Phases {
		var status string
		switch phase.Status {
		case "failed":
			status = "[FAILED]"
		case "skipped":
			status = "[SKIPPED]"
		case "dry-run":
			status = "[DRY-RUN]"
		default:
			status = "[SUCCESS]"
		}

		phaseName := formatPhaseName(phase.Phase)
		_, _ = fmt.Fprintf(w.out, "Phase: %-20s %s\n", phaseName, status)

		if phase.Error != nil {
			_, _ = fmt.Fprintf(w.out, "  Error: %v\n", phase.Error)
		}

		if w.verbose && len(phase.Details) > 0 {
			for key, val := range phase.Details {
				_, _ = fmt.Fprintf(w.out, "  %s: %v\n", key, val)
			}
		}
		_, _ = fmt.Fprintln(w.out)
	}

	// Show rendered outputs if available
	if result.RenderedOutputs != nil {
		if len(result.RenderedOutputs.Manifests) > 0 {
			_, _ = fmt.Fprintln(w.out, "Rendered Manifests:")
			for name, manifest := range result.RenderedOutputs.Manifests {
				_, _ = fmt.Fprintf(w.out, "  [%s]\n", name)
				if w.verbose {
					for _, line := range strings.Split(manifest, "\n") {
						_, _ = fmt.Fprintf(w.out, "    %s\n", line)
					}
				}
			}
			_, _ = fmt.Fprintln(w.out)
		}

		if len(result.RenderedOutputs.APICalls) > 0 {
			_, _ = fmt.Fprintln(w.out, "API Calls (simulated):")
			for _, call := range result.RenderedOutputs.APICalls {
				_, _ = fmt.Fprintf(w.out, "  %s %s\n", call.Method, call.URL)
				if w.verbose {
					if call.Body != "" {
						_, _ = fmt.Fprintf(w.out, "    Request Body: %s\n", truncate(call.Body, 200))
					}
					_, _ = fmt.Fprintf(w.out, "    Response [%d]: %s\n", call.StatusCode, truncate(call.Response, 200))
				}
			}
		}
	}

	return nil
}

func (w *textOutputWriter) WriteError(err error) error {
	_, _ = fmt.Fprintf(w.out, "Error: %v\n", err)
	return nil
}

func formatPhaseName(phase executor.ExecutionPhase) string {
	switch phase {
	case executor.PhaseParamExtraction:
		return "Parameter Extraction"
	case executor.PhasePreconditions:
		return "Preconditions"
	case executor.PhaseResources:
		return "Resources"
	case executor.PhasePostActions:
		return "Post Actions"
	default:
		return string(phase)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// jsonOutputWriter writes JSON output
type jsonOutputWriter struct {
	out io.Writer
}

func (w *jsonOutputWriter) WriteValidationResult(result *ValidationResult) error {
	return w.writeJSON(result)
}

func (w *jsonOutputWriter) WriteTraceResult(result *TraceResult) error {
	return w.writeJSON(result)
}

func (w *jsonOutputWriter) WriteError(err error) error {
	return w.writeJSON(map[string]string{"error": err.Error()})
}

func (w *jsonOutputWriter) writeJSON(v interface{}) error {
	enc := json.NewEncoder(w.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
