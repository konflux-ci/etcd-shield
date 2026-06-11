# etcd-shield

Kubernetes admission webhook that blocks Tekton PipelineRun creation when etcd storage is near capacity. Uses Prometheus queries and a JK flip-flop state machine with separate set/reset thresholds to prevent admission spam.

## Build & Test Commands

| Action | Command |
|---|---|
| Build binary | `make build` |
| Build image | `make build-image` |
| Run tests | `make test` |
| Test with coverage | `make test-coverage` |
| Lint all (Go + YAML) | `make lint` |
| Lint Go only | `make lint-go` |
| Lint YAML only | `make lint-yaml` |
| Format | `make fmt` |
| Vet | `make vet` |

### Single-File Verification

- Lint: `go run -modfile hack/tools/golang-ci/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint run ./path/to/package/...`
- Vet: `go vet ./pkg/specific/...`
- Test: `go test ./pkg/specific/...`
- Test with race: `go test -race ./pkg/specific/...`
- Format: `gofmt -w path/to/file.go`
- YAML lint: `yamllint path/to/file.yaml`

### K8s Manifests

| Action | Command |
|---|---|
| Build | `kustomize build config/` |
| Dry-run | `kustomize build config/ \| kubectl apply --dry-run=client -f -` |

## Project Layout

- `cmd/etcd-shield/` — entry point (`main.go`) and coverage init
- `pkg/` — core logic: webhook handler, Prometheus querier, state machine, config, metrics
- `config/` — Kubernetes manifests (deployment, webhook, RBAC, ConfigMap, Prometheus alerts)
- `config/metrics/` — ServiceMonitor, NetworkPolicy for metrics scraping
- `acceptance/` — Chainsaw e2e tests
- `hack/` — test scripts (`e2e_tests.sh`), golangci-lint pinned version
- `.tekton/` — Konflux pipeline definitions (pull-request, push)
- `.github/workflows/` — CI linting and tests (`ci.yaml`), Chainsaw e2e (`chainsaw-tests.yaml`), auto-merge, dep-triage
- `Dockerfile` — multi-stage build (UBI10 Go toolset → UBI10 ubi-micro)
- `renovate.json` — MintMaker/Renovate config for Go, K8s, Docker, and GitHub Actions updates

## Key Conventions

- Go lint (`make lint`) must pass before changes are accepted.
- All changes via PR.
- Tests use **Ginkgo v2 + Gomega** BDD framework.
- golangci-lint version is pinned in `hack/tools/golang-ci/go.mod` (currently v2.10.1).
- Dockerfile uses `CGO_ENABLED=0` for a static binary, runs as non-root (UID 65532).
- Coverage is collected for both unit tests (codecov) and e2e tests (coverport).

## Gotchas

- `config/` manifests are not directly deployed to any cluster — they're reference configs used by kustomize overlays in deployment repos.
- State tracking uses Prometheus recording rules with JK flip-flop logic — the webhook itself is stateless.
- The `main` binary in the repo root is a build artifact that should not be committed (listed in `.gitignore`).
- `.yamllint.yaml` ignores `.tekton/` and allows 120-char lines.

See `skills/` for detailed guides:
- `running-tests/` — unit tests, e2e tests, coverage, Chainsaw
- `building-and-linting/` — local build, golangci-lint setup, Dockerfile
- `ci-cd-quirks/` — Tekton pipelines, GitHub Actions, Renovate config
