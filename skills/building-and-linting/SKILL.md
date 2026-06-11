---
name: building-and-linting
description: Use when building the binary or container image, running linters, or troubleshooting build failures. Covers Makefile targets, golangci-lint setup, Dockerfile structure, and YAML linting.
---

# Building and Linting

## Quick Reference

| Action | Command |
|--------|---------|
| Build binary | `make build` |
| Build image | `make build-image` |
| Lint all | `make lint` |
| Lint Go | `make lint-go` |
| Lint YAML | `make lint-yaml` |
| Format | `make fmt` |
| Vet | `make vet` |

## golangci-lint Setup

golangci-lint is **not installed globally** — it's pinned in `hack/tools/golang-ci/go.mod` (currently v2.10.1) and invoked via `go run -modfile`:

```bash
go run -modfile hack/tools/golang-ci/go.mod \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint run
```

`make lint-go` does this for you. There is no `.golangci.yml` config file — the linter runs with default settings.

## Dockerfile

Two-stage build:
1. **Builder** (`ubi10/go-toolset`): compiles `cmd/etcd-shield/main.go` with `CGO_ENABLED=0`
2. **Runtime** (`ubi10/ubi-micro`): copies `/tmp/server` binary, runs as UID 65532

The `ENABLE_COVERAGE` build arg enables coverage instrumentation for e2e test collection.

Note: this project uses `Dockerfile`, not `Containerfile`.

## YAML Linting

`.yamllint.yaml` config:
- Extends `relaxed` preset
- Ignores `.tekton/` directory (Tekton manifests are too long)
- Line length limit: 120 characters

## Common Build Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| `golangci-lint: command not found` | Not installed globally | Use `make lint-go` (runs via `go run -modfile`) |
| YAML lint fails on Tekton files | `.tekton/` not excluded | Check `.yamllint.yaml` has `ignore: [.tekton/]` |
| Image build uses Docker not Podman | `IMAGE_BUILDER` defaults to `podman` | Override: `make build-image IMAGE_BUILDER=docker` |
