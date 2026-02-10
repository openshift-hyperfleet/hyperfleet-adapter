package manifest

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ValidateManifest validates a Kubernetes manifest has all required fields and annotations
func ValidateManifest(obj *unstructured.Unstructured) error {
	if obj.GetAPIVersion() == "" {
		return fmt.Errorf("manifest missing apiVersion")
	}
	if obj.GetKind() == "" {
		return fmt.Errorf("manifest missing kind")
	}
	if obj.GetName() == "" {
		return fmt.Errorf("manifest missing metadata.name")
	}

	if GetGenerationFromUnstructured(obj) == 0 {
		return fmt.Errorf("manifest missing required annotation %q", constants.AnnotationGeneration)
	}

	return nil
}
