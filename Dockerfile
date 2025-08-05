# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset@sha256:3ce6311380d5180599a3016031a9112542d43715244816d1d0eabc937952667b AS builder
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

FROM registry.access.redhat.com/ubi9/ubi-micro@sha256:647c02b5b6d4760b019a468ff620ec53009d8453679bee3b7e5a676499edb1c3
WORKDIR /
COPY --from=builder /tmp/server .
USER 65532:65532

ENTRYPOINT ["/server"]

