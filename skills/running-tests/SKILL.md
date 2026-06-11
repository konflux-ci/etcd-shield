---
name: running-tests
description: Use when running, writing, or troubleshooting unit tests or e2e tests for etcd-shield. Covers make test, Ginkgo patterns, Chainsaw acceptance tests, coverage collection, and common test failures.
---

# Running Tests

## Quick Reference

| Action | Command |
|--------|---------|
| Unit tests | `make test` |
| Unit tests with coverage | `make test-coverage` |
| Single package | `go test ./pkg/specific/...` |
| With race detector | `go test -race ./pkg/specific/...` |
| E2e tests (full setup) | `hack/e2e_tests.sh` |

## Unit Tests

Tests use **Ginkgo v2 + Gomega** BDD framework. Each package has a `suite_test.go` that registers the Ginkgo runner. Property-based tests use `pgregory.net/rapid`.

Coverage is uploaded to Codecov with flags `unit-tests` and `e2e-tests` (carryforward enabled). The coverage target range is 50-100% (configured in `codecov.yml`).

## E2e Tests (Chainsaw)

End-to-end tests use [Chainsaw](https://kyverno.github.io/chainsaw/) and require a full environment:

1. KinD cluster
2. Tekton installed
3. cert-manager installed
4. Prometheus installed
5. etcd-shield built and deployed

The `hack/e2e_tests.sh` script handles all of this. The `chainsaw-tests.yaml` GitHub Action runs this in CI.

The Dockerfile supports coverage instrumentation for e2e tests via the `ENABLE_COVERAGE` build arg — set it to collect coverage from the running binary via coverport.

## Common Test Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| Ginkgo "no test suites" | Missing `suite_test.go` in package | Add `RunSpecs` suite file |
| Chainsaw tests fail locally | Missing Tekton/cert-manager/Prometheus | Run `hack/e2e_tests.sh` for full setup |
| Coverage mismatch in CI | Unit and e2e flags separate | Check `codecov.yml` flag configuration |
