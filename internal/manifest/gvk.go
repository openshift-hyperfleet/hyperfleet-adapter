package manifest

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ExtractGVKFromString extracts apiVersion and kind from a YAML string
// by scanning lines. This handles manifests with Go template directives
// that would fail full YAML parsing.
//
// Assumption: apiVersion and kind are always static (not templated).
// This is a valid K8s constraint — the API server requires exact schema
// identifiers. This function is also used in deletion flows where no
// rendered manifest is available (discover → delete, no render/apply).
func ExtractGVKFromString(manifest string) schema.GroupVersionKind {
	var apiVersion, kind string
	for line := range strings.SplitSeq(manifest, "\n") {
		trimmed := strings.TrimSpace(line)
		if val, ok := strings.CutPrefix(trimmed, "apiVersion:"); ok {
			apiVersion = strings.TrimSpace(val)
			apiVersion = strings.Trim(apiVersion, "\"'")
		} else if val, ok := strings.CutPrefix(trimmed, "kind:"); ok {
			kind = strings.TrimSpace(val)
			kind = strings.Trim(kind, "\"'")
		}
		if apiVersion != "" && kind != "" {
			break
		}
	}
	if apiVersion == "" || kind == "" {
		return schema.GroupVersionKind{}
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}
	}
	return gv.WithKind(kind)
}
