# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:ad1d5e19331fc80c28a6193c1f8489af93b8f54d06766f174de6d4ce1ec6a191 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false

USER 0
WORKDIR /build

# Copy the Go Modules manifests
COPY go.mod go.sum /build
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# copy source
COPY . .

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "Building with coverage instrumentation..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=coverage -o /tmp/server ./cmd/etcd-shield/; \
    else \
        echo "Building production binary..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -trimpath -a -o /tmp/server ./cmd/etcd-shield/; \
    fi

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:e025b421d29b8ecd86fcfd8a4ad18c0d7de747cbcc4e372f414ba02390b031b6
WORKDIR /
COPY --from=builder /tmp/server .
USER 65532:65532

ENTRYPOINT ["/server"]

