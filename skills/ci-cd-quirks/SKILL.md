---
name: ci-cd-quirks
description: Use when CI checks fail unexpectedly, when modifying Tekton pipelines or GitHub Actions, or when debugging build or test failures in CI. Covers pipeline structure, GitHub Actions workflows, Renovate config, and hermetic builds.
---

# CI/CD Quirks

## Pipeline Structure

| Pipeline | Trigger | Purpose |
|----------|---------|---------|
| `.tekton/etcd-shield-pull-request.yaml` | PR opened/updated | Build + security scans on PR |
| `.tekton/etcd-shield-push.yaml` | Merge to main | Production build + push to registry |

**Container builds run in Konflux, not GitHub Actions.** Tekton pipelines use Cachi2 for hermetic builds and include SAST scans (Snyk, Coverity, shell, unicode), Clair vulnerability scanning, and image signing via sigstore.

## GitHub Actions Workflows

| Workflow | Trigger | What It Does |
|----------|---------|--------------|
| `ci.yaml` | PR + push to main | Unit tests (with codecov), `go mod tidy` check, `go vet`, golangci-lint, yamllint |
| `chainsaw-tests.yaml` | PR + push to main | Full e2e: KinD cluster, Tekton, cert-manager, Prometheus, Chainsaw tests |
| `auto-merge.yaml` | Bot PRs | Auto-merges approved Renovate/MintMaker PRs |
| `dep-triage.yaml` | Bot PRs | Dependency impact analysis (uses Gemini API), auto-approve/merge |

## Renovate / MintMaker

Renovate (`renovate.json`) manages:
- **Go module updates** — grouped by minor/patch vs major; K8s packages (`k8s.io/*`) grouped together
- **Dockerfile base image updates** — digest pinning
- **GitHub Actions updates** — via dependabot (separate config in `.github/dependabot.yml`)
- Schedule: before 7am Monday

## Hermetic Builds

Tekton pipelines use Cachi2 for network-isolated builds. When adding Go dependencies:

```bash
go mod tidy
# Cachi2 prefetches from go.sum — ensure it's up to date
```

No lockfiles beyond `go.sum` are needed (unlike repos with `rpms.lock.yaml` or `artifacts.lock.yaml`).

## Common CI Failures

| Symptom | Cause | Fix |
|---------|-------|-----|
| `go mod tidy` check fails | `go.sum` out of sync | Run `go mod tidy` and commit both `go.mod` and `go.sum` |
| golangci-lint version mismatch | CI uses version from `hack/tools/golang-ci/go.mod` | Update the pinned version there, not a global install |
| Chainsaw e2e timeout | KinD cluster setup is slow | Check resource limits; the full e2e stack is heavy |
| yamllint fails | `.tekton/` not excluded from check | Verify `.yamllint.yaml` has `ignore: [.tekton/]` |
