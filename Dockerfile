# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset@sha256:2e8adc5be8eba060e919a6f7309a7b5ef09dd72d3ae9e747b9eb02e96f35a0f5 AS builder
ARG TARGETOS
ARG TARGETARCH

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
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -trimpath -a -o /tmp/server ./cmd/etcd-shield/main.go

FROM registry.access.redhat.com/ubi9/ubi-micro@sha256:e14a8cbcaa0c26b77140ac85d40a47b5e910a4068686b02ebcad72126e9b5f86
WORKDIR /
COPY --from=builder /tmp/server .
USER 65532:65532

ENTRYPOINT ["/server"]

