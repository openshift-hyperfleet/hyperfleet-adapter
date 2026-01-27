package executor

// StepType identifies the kind of step being executed
type StepType string

const (
	// StepTypeParam indicates a parameter step
	StepTypeParam StepType = "param"
	// StepTypeAPICall indicates an API call step
	StepTypeAPICall StepType = "apiCall"
	// StepTypeResource indicates a Kubernetes resource step
	StepTypeResource StepType = "resource"
	// StepTypePayload indicates a payload builder step
	StepTypePayload StepType = "payload"
	// StepTypeLog indicates a logging step
	StepTypeLog StepType = "log"
)

// StepResult represents the result of executing a single step.
// Each step produces a result that can be accessed by step name:
//   - stepName returns the Result value directly (for convenience)
//   - stepName.error returns the StepError if the step failed
//   - stepName.skipped returns true if the step was skipped due to 'when' clause
type StepResult struct {
	// Name is the step name from config
	Name string `json:"name"`
	// Type is the type of step that was executed
	Type StepType `json:"type"`
	// Result is the step-specific result value:
	//   - param: the extracted/computed value
	//   - apiCall: the parsed API response (map[string]interface{})
	//   - resource: the K8s resource object (map[string]interface{})
	//   - payload: the built payload (map[string]interface{})
	//   - log: nil
	Result interface{} `json:"result,omitempty"`
	// Error contains error details if the step failed
	Error *StepError `json:"error,omitempty"`
	// Skipped is true if the step was skipped due to 'when' clause evaluating to false
	Skipped bool `json:"skipped"`
	// SkipReason provides details when Skipped is true
	SkipReason string `json:"skipReason,omitempty"`
}

// StepError represents an error that occurred during step execution.
// Errors are soft failures - execution continues to subsequent steps.
// Use stepName.error in 'when' clauses to check for errors.
type StepError struct {
	// Reason is a short error code (e.g., "APICallFailed", "ParamExtractionFailed")
	Reason string `json:"reason"`
	// Message is a human-readable error message
	Message string `json:"message"`
}

// NewStepResult creates a new successful step result
func NewStepResult(name string, stepType StepType, result interface{}) *StepResult {
	return &StepResult{
		Name:   name,
		Type:   stepType,
		Result: result,
	}
}

// NewStepResultSkipped creates a new skipped step result
func NewStepResultSkipped(name string, stepType StepType, reason string) *StepResult {
	return &StepResult{
		Name:       name,
		Type:       stepType,
		Skipped:    true,
		SkipReason: reason,
	}
}

// NewStepResultError creates a new error step result
func NewStepResultError(name string, stepType StepType, reason, message string) *StepResult {
	return &StepResult{
		Name: name,
		Type: stepType,
		Error: &StepError{
			Reason:  reason,
			Message: message,
		},
	}
}

// IsSuccess returns true if the step completed successfully (not skipped, no error)
func (r *StepResult) IsSuccess() bool {
	return !r.Skipped && r.Error == nil
}

// ToMap converts the step result to a map for CEL evaluation.
// The map includes:
//   - result: the step result value (or nil)
//   - error: the error object (or nil)
//   - skipped: boolean indicating if step was skipped
func (r *StepResult) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"skipped": r.Skipped,
	}

	if r.Result != nil {
		m["result"] = r.Result
	}

	if r.Error != nil {
		m["error"] = map[string]interface{}{
			"reason":  r.Error.Reason,
			"message": r.Error.Message,
		}
	} else {
		m["error"] = nil
	}

	return m
}

// StepExecutionResult contains the overall result of step-based execution
type StepExecutionResult struct {
	// Status is the overall execution status
	Status ExecutionStatus
	// StepResults contains results of all executed steps in order
	StepResults []*StepResult
	// StepResultsByName provides quick lookup of step results by name
	StepResultsByName map[string]*StepResult
	// Variables contains all variables available at the end of execution
	Variables map[string]interface{}
	// HasErrors indicates if any step had an error
	HasErrors bool
	// FirstError is the first error that occurred (if any)
	FirstError *StepError
}

// NewStepExecutionResult creates a new step execution result
func NewStepExecutionResult() *StepExecutionResult {
	return &StepExecutionResult{
		Status:            StatusSuccess,
		StepResults:       make([]*StepResult, 0),
		StepResultsByName: make(map[string]*StepResult),
		Variables:         make(map[string]interface{}),
	}
}

// AddStepResult adds a step result to the execution result
func (r *StepExecutionResult) AddStepResult(result *StepResult) {
	r.StepResults = append(r.StepResults, result)
	r.StepResultsByName[result.Name] = result

	if result.Error != nil {
		r.HasErrors = true
		if r.FirstError == nil {
			r.FirstError = result.Error
		}
	}
}

// GetStepResult returns a step result by name
func (r *StepExecutionResult) GetStepResult(name string) *StepResult {
	return r.StepResultsByName[name]
}
