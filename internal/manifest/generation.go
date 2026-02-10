// Package manifest provides generation-based resource tracking, manifest validation,
// and rendering utilities for the transport abstraction layer.
package manifest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	apperrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	workv1 "open-cluster-management.io/api/work/v1"
)

// Operation represents the type of operation to perform on a resource
type Operation string

const (
	OperationCreate   Operation = "create"
	OperationUpdate   Operation = "update"
	OperationRecreate Operation = "recreate"
	OperationSkip     Operation = "skip"
)

// ApplyDecision contains the decision about what operation to perform
type ApplyDecision struct {
	Operation          Operation
	Reason             string
	NewGeneration      int64
	ExistingGeneration int64
}

// CompareGenerations compares the generation of a new resource against an existing one
func CompareGenerations(newGen, existingGen int64, exists bool) ApplyDecision {
	if !exists {
		return ApplyDecision{
			Operation:          OperationCreate,
			Reason:             "resource not found",
			NewGeneration:      newGen,
			ExistingGeneration: 0,
		}
	}

	if existingGen == newGen {
		return ApplyDecision{
			Operation:          OperationSkip,
			Reason:             fmt.Sprintf("generation %d unchanged", existingGen),
			NewGeneration:      newGen,
			ExistingGeneration: existingGen,
		}
	}

	return ApplyDecision{
		Operation:          OperationUpdate,
		Reason:             fmt.Sprintf("generation changed %d->%d", existingGen, newGen),
		NewGeneration:      newGen,
		ExistingGeneration: existingGen,
	}
}

// GetGeneration extracts the generation annotation value from ObjectMeta.
func GetGeneration(meta metav1.ObjectMeta) int64 {
	if meta.Annotations == nil {
		return 0
	}

	genStr, ok := meta.Annotations[constants.AnnotationGeneration]
	if !ok || genStr == "" {
		return 0
	}

	gen, err := strconv.ParseInt(genStr, 10, 64)
	if err != nil {
		return 0
	}

	return gen
}

// GetGenerationFromUnstructured is a convenience wrapper for getting generation from unstructured.Unstructured.
func GetGenerationFromUnstructured(obj *unstructured.Unstructured) int64 {
	if obj == nil {
		return 0
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return 0
	}
	genStr, ok := annotations[constants.AnnotationGeneration]
	if !ok || genStr == "" {
		return 0
	}
	gen, err := strconv.ParseInt(genStr, 10, 64)
	if err != nil {
		return 0
	}
	return gen
}

// ValidateGeneration validates that the generation annotation exists and is valid on ObjectMeta.
func ValidateGeneration(meta metav1.ObjectMeta) error {
	if meta.Annotations == nil {
		return apperrors.Validation("missing %s annotation", constants.AnnotationGeneration).AsError()
	}

	genStr, ok := meta.Annotations[constants.AnnotationGeneration]
	if !ok {
		return apperrors.Validation("missing %s annotation", constants.AnnotationGeneration).AsError()
	}

	if genStr == "" {
		return apperrors.Validation("%s annotation is empty", constants.AnnotationGeneration).AsError()
	}

	gen, err := strconv.ParseInt(genStr, 10, 64)
	if err != nil {
		return apperrors.Validation("invalid %s annotation value %q: %v", constants.AnnotationGeneration, genStr, err).AsError()
	}

	if gen <= 0 {
		return apperrors.Validation("%s annotation must be > 0, got %d", constants.AnnotationGeneration, gen).AsError()
	}

	return nil
}

// ValidateManifestWorkGeneration validates that the generation annotation exists on both
// the ManifestWork metadata and all manifests within the workload.
func ValidateManifestWorkGeneration(work *workv1.ManifestWork) error {
	if work == nil {
		return apperrors.Validation("work cannot be nil").AsError()
	}

	if err := ValidateGeneration(work.ObjectMeta); err != nil {
		return apperrors.Validation("ManifestWork %q: %v", work.Name, err).AsError()
	}

	for i, m := range work.Spec.Workload.Manifests {
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(m.Raw); err != nil {
			return apperrors.Validation("ManifestWork %q manifest[%d]: failed to unmarshal: %v", work.Name, i, err).AsError()
		}

		if err := ValidateGenerationFromUnstructured(obj); err != nil {
			kind := obj.GetKind()
			name := obj.GetName()
			return apperrors.Validation("ManifestWork %q manifest[%d] %s/%s: %v", work.Name, i, kind, name, err).AsError()
		}
	}

	return nil
}

// ValidateGenerationFromUnstructured validates that the generation annotation exists and is valid on an Unstructured object.
func ValidateGenerationFromUnstructured(obj *unstructured.Unstructured) error {
	if obj == nil {
		return apperrors.Validation("object cannot be nil").AsError()
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return apperrors.Validation("missing %s annotation", constants.AnnotationGeneration).AsError()
	}

	genStr, ok := annotations[constants.AnnotationGeneration]
	if !ok {
		return apperrors.Validation("missing %s annotation", constants.AnnotationGeneration).AsError()
	}

	if genStr == "" {
		return apperrors.Validation("%s annotation is empty", constants.AnnotationGeneration).AsError()
	}

	gen, err := strconv.ParseInt(genStr, 10, 64)
	if err != nil {
		return apperrors.Validation("invalid %s annotation value %q: %v", constants.AnnotationGeneration, genStr, err).AsError()
	}

	if gen <= 0 {
		return apperrors.Validation("%s annotation must be > 0, got %d", constants.AnnotationGeneration, gen).AsError()
	}

	return nil
}

// GetLatestGenerationFromList returns the resource with the highest generation annotation from a list.
func GetLatestGenerationFromList(list *unstructured.UnstructuredList) *unstructured.Unstructured {
	if list == nil || len(list.Items) == 0 {
		return nil
	}

	items := make([]unstructured.Unstructured, len(list.Items))
	copy(items, list.Items)

	sort.Slice(items, func(i, j int) bool {
		genI := GetGenerationFromUnstructured(&items[i])
		genJ := GetGenerationFromUnstructured(&items[j])
		if genI != genJ {
			return genI > genJ
		}
		return items[i].GetName() < items[j].GetName()
	})

	return &items[0]
}

// DiscoveryConfig is the default implementation of the Discovery interface.
type DiscoveryConfig struct {
	Namespace     string
	ByName        string
	LabelSelector string
}

// GetNamespace implements Discovery.GetNamespace
func (d *DiscoveryConfig) GetNamespace() string { return d.Namespace }

// GetName implements Discovery.GetName
func (d *DiscoveryConfig) GetName() string { return d.ByName }

// GetLabelSelector implements Discovery.GetLabelSelector
func (d *DiscoveryConfig) GetLabelSelector() string { return d.LabelSelector }

// IsSingleResource implements Discovery.IsSingleResource
func (d *DiscoveryConfig) IsSingleResource() bool { return d.ByName != "" }

// BuildLabelSelector converts a map of labels to a selector string.
func BuildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(labels))
	for _, k := range keys {
		pairs = append(pairs, k+"="+labels[k])
	}
	return strings.Join(pairs, ",")
}
