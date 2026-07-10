module github.com/openshift-hyperfleet/hyperfleet-adapter/tools

go 1.26.0

tool (
	github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	github.com/norwoodj/helm-docs/cmd/helm-docs
	github.com/yannh/kubeconform/cmd/kubeconform
	golang.org/x/tools/cmd/goimports
)

require (
	github.com/golangci/golangci-lint/v2 v2.7.0
	github.com/norwoodj/helm-docs v1.14.2
	github.com/yannh/kubeconform v0.8.0
	golang.org/x/tools v0.47.0
)
