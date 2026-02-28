ARG BASE_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

ARG GIT_SHA=unknown
ARG GIT_DIRTY=""
ARG BUILD_DATE=""
ARG VERSION=""

# Install make as root (UBI9 go-toolset doesn't include it), then switch back to non-root.
USER root
RUN dnf install -y make && dnf clean all
WORKDIR /build
RUN chown 1001:0 /build
USER 1001

ENV GOBIN=/build/.gobin

COPY --chown=1001:0 go.mod go.sum ./
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    go mod download

COPY --chown=1001:0 . .

# CGO_ENABLED=0 produces a static binary required for distroless runtime.
# For FIPS-compliant builds (CGO_ENABLED=1 + GOEXPERIMENT=boringcrypto), use a
# runtime image with glibc (e.g. ubi9-micro) instead of distroless.
RUN --mount=type=cache,target=/opt/app-root/src/go/pkg/mod,uid=1001 \
    --mount=type=cache,target=/opt/app-root/src/.cache/go-build,uid=1001 \
    CGO_ENABLED=0 GOOS=linux \
    GIT_SHA=${GIT_SHA} GIT_DIRTY=${GIT_DIRTY} BUILD_DATE=${BUILD_DATE} VERSION=${VERSION} \
    make build

# Runtime stage
FROM ${BASE_IMAGE}

WORKDIR /app

COPY --from=builder /build/bin/hyperfleet-adapter /app/adapter

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/app/adapter"]
CMD ["serve"]

ARG VERSION=""
LABEL name="hyperfleet-adapter" \
      vendor="Red Hat" \
      version="${VERSION}" \
      summary="HyperFleet Adapter - Event-driven adapter services for HyperFleet cluster provisioning" \
      description="Handles CloudEvents consumption, AdapterConfig CRD integration, precondition evaluation, Kubernetes Job creation/monitoring, and status reporting via API"
