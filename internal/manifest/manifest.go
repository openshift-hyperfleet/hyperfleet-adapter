package manifest

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apperrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
)

func ValidateManifest(obj *unstructured.Unstructured) error {
	if obj == nil {
		return apperrors.Validation("manifest cannot be nil").AsError()
	}

	// Validate required Kubernetes fields
	if obj.GetAPIVersion() == "" {
		return apperrors.Validation("manifest missing apiVersion").AsError()
	}
	if obj.GetKind() == "" {
		return apperrors.Validation("manifest missing kind").AsError()
	}
	if obj.GetName() == "" {
		return apperrors.Validation("manifest missing metadata.name").AsError()
	}

	// Validate required generation annotation
	if err := ValidateGenerationFromUnstructured(obj); err != nil {
		return err
	}

	return nil
}
