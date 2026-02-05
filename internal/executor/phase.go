package executor

import (
	"context"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// Phase defines the interface for execution phases.
// Each phase is responsible for a specific part of the execution pipeline.
type Phase interface {
	// Name returns the phase identifier
	Name() ExecutionPhase
	// Execute runs the phase logic
	Execute(ctx context.Context, execCtx *ExecutionContext) error
	// ShouldSkip determines if this phase should be skipped based on execution context
	ShouldSkip(execCtx *ExecutionContext) (skip bool, reason string)
}

// PhaseResult contains the outcome of a single phase execution
type PhaseResult struct {
	// Phase is the phase identifier
	Phase ExecutionPhase
	// Status is the execution status (success/failed)
	Status ExecutionStatus
	// Error contains any error that occurred during execution
	Error error
	// Skipped indicates if the phase was skipped
	Skipped bool
	// SkipReason explains why the phase was skipped
	SkipReason string
	// Duration is how long the phase took to execute
	Duration time.Duration
}

// Pipeline executes phases in sequence
type Pipeline struct {
	phases []Phase
	log    logger.Logger
}

// NewPipeline creates a new execution pipeline with the given phases
func NewPipeline(log logger.Logger, phases ...Phase) *Pipeline {
	return &Pipeline{
		phases: phases,
		log:    log,
	}
}

// Execute runs all phases in sequence and returns results for each phase.
// The pipeline stops on error for most phases, except post-actions which always run.
func (p *Pipeline) Execute(ctx context.Context, execCtx *ExecutionContext) []PhaseResult {
	var results []PhaseResult
	var shouldStopExecution bool

	for _, phase := range p.phases {
		start := time.Now()
		phaseName := phase.Name()

		// Check if phase should be skipped
		if skip, reason := phase.ShouldSkip(execCtx); skip {
			p.log.Infof(ctx, "Phase %s: SKIPPED - %s", phaseName, reason)
			results = append(results, PhaseResult{
				Phase:      phaseName,
				Status:     StatusSuccess,
				Skipped:    true,
				SkipReason: reason,
				Duration:   time.Since(start),
			})
			continue
		}

		// Skip execution if a previous phase failed (except post-actions always run)
		if shouldStopExecution && phaseName != PhasePostActions {
			p.log.Infof(ctx, "Phase %s: SKIPPED - previous phase failed", phaseName)
			results = append(results, PhaseResult{
				Phase:      phaseName,
				Status:     StatusSuccess,
				Skipped:    true,
				SkipReason: "previous phase failed",
				Duration:   time.Since(start),
			})
			continue
		}

		// Execute the phase
		p.log.Infof(ctx, "Phase %s: RUNNING", phaseName)
		err := phase.Execute(ctx, execCtx)
		duration := time.Since(start)

		result := PhaseResult{
			Phase:    phaseName,
			Error:    err,
			Duration: duration,
		}

		if err != nil {
			result.Status = StatusFailed
			p.log.Errorf(logger.WithErrorField(ctx, err), "Phase %s: FAILED", phaseName)
			// Mark that execution should stop for subsequent phases (except post-actions)
			shouldStopExecution = true
		} else {
			result.Status = StatusSuccess
			p.log.Infof(ctx, "Phase %s: SUCCESS", phaseName)
		}

		results = append(results, result)
	}

	return results
}

// GetPhase returns a phase by name, or nil if not found
func (p *Pipeline) GetPhase(name ExecutionPhase) Phase {
	for _, phase := range p.phases {
		if phase.Name() == name {
			return phase
		}
	}
	return nil
}

// Phases returns all phases in the pipeline
func (p *Pipeline) Phases() []Phase {
	return p.phases
}
