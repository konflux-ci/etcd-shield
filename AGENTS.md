# etcd-shield

Block admission of pipelineruns when etcd fills up using a validation webhook.

## Quick Commands

Go service:
| Action             | Command              |
|--------------------|----------------------|
| Build              | `make build`         |
| Format             | `make fmt`           |
| Image              | `make build-image`   |
| Lint all           | `make lint`          |
| Lint go            | `make lint-go`       |
| Test               | `make test`          |
| Test with coverage | `make test-coverage` |

### Single-File Verification

- Lint single file: `go run -modfile hack/tools/golang-ci/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint run ./path/to/package/...`
- Vet single package: `go vet ./pkg/specific/...`
- Test single package: `go test ./pkg/specific/...`
- Test with race detector: `go test -race ./pkg/specific/...`
- Format single file: `gofmt -w path/to/file.go`
- YAML lint single file: `yamllint path/to/file.yaml`

K8s manifests:
| Action    | Command                                                          |
|-----------|------------------------------------------------------------------|
| Build     | `kustomize build config/`                                        |
| Dry-run   | `kustomize build config/ \| kubectl apply --dry-run=client -f -` |
| Lint YAML | `make lint-yaml`                                                 |

## Project Layout

Go service:
- `cmd/` — entry points
- `pkg/` — core logic
- `config/` — Kubernetes manifests

## Key Conventions

- Go: lint checks must pass before changes will be accepted.
- All changes must be submitted via PR.

## Gotchas

- `config/` is not directly deployed to any cluster.
- State tracking of allowing or denying admission is achieved entirely using prometheus recording rules.
