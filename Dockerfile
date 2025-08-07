# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset@sha256:3f552f246b4bd5bdfb4da0812085d381d00d3625769baecaed58c2667d344e5c AS builder
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

FROM registry.access.redhat.com/ubi9/ubi-micro@sha256:f5c5213d2969b7b11a6666fc4b849d56b48d9d7979b60a37bb853dff0255c14b
WORKDIR /
COPY --from=builder /tmp/server .
USER 65532:65532

ENTRYPOINT ["/server"]

