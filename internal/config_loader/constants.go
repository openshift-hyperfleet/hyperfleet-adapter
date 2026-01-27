package config_loader

// Field path constants for configuration structure.
// These constants define the known field names used in adapter configuration
// to avoid hardcoding strings throughout the codebase.

// Top-level field names
const (
	FieldSpec     = "spec"
	FieldMetadata = "metadata"
)

// Spec section field names
const (
	FieldAdapter       = "adapter"
	FieldHyperfleetAPI = "hyperfleetApi"
	FieldKubernetes    = "kubernetes"
	FieldSteps         = "steps"
)

// Adapter field names
const (
	FieldVersion = "version"
)

// Step field names
const (
	FieldName       = "name"
	FieldWhen       = "when"
	FieldParam      = "param"
	FieldAPICall    = "apiCall"
	FieldResource   = "resource"
	FieldPayload    = "payload"
	FieldLog        = "log"
	FieldDefault    = "default"
	FieldSource     = "source"
	FieldExpression = "expression"
	FieldValue      = "value"
	FieldCapture    = "capture"
)

// API call field names
const (
	FieldMethod  = "method"
	FieldURL     = "url"
	FieldTimeout = "timeout"
	FieldHeaders = "headers"
	FieldBody    = "body"
)

// Resource field names
const (
	FieldManifest         = "manifest"
	FieldRecreateOnChange = "recreateOnChange"
	FieldDiscovery        = "discovery"
)

// Discovery field names
const (
	FieldNamespace   = "namespace"
	FieldByName      = "byName"
	FieldBySelectors = "bySelectors"
)

// Selector field names
const (
	FieldLabelSelector = "labelSelector"
)

// Kubernetes manifest field names
const (
	FieldAPIVersion = "apiVersion"
	FieldKind       = "kind"
)
