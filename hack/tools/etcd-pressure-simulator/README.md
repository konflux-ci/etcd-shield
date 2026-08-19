# etcd-pressure-simulator

A local etcd storage pressure simulator for KinD clusters.

This tool creates real Kubernetes ConfigMaps that consume real etcd storage.
Prometheus observes the actual `etcd_mvcc_db_total_size_in_bytes` metric
through its normal scraping mechanism. The tool provides four commands:

- **`status`** — shows current etcd physical usage, in-use size, quota, and
  pressure ConfigMap count.
- **`fill <percent>`** — adds ConfigMaps until Prometheus reports the requested
  usage level.
- **`drain <percent>`** — gradually removes ConfigMaps until Prometheus reports
  the requested usage level. The metric decreases monotonically and never drops
  below a configurable safety floor (default 81%).
- **`cleanup`** — removes all resources created by this tool and returns etcd
  close to its baseline state.

Internally, `fill` and `drain` run etcd compaction and defragmentation between
steps because etcd's storage file does not shrink on delete alone.

## What this tool does not test

This tool only creates the etcd pressure states that E2E tests need. It does
not:

- Deploy or modify the etcd-shield PrometheusRule.
- Verify that an alert is firing.
- Create or verify PipelineRuns.
- Verify admission allow or deny behavior.
- Run Chainsaw tests.
- Modify CI configuration.

E2E developers should add their own assertions between the tool commands.

## Prerequisites

| Tool | Notes |
|------|-------|
| Bash | Default shell on Linux and macOS |
| kubectl | Must be able to access the local KinD cluster |
| [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) | May need separate installation (`go install sigs.k8s.io/kind@latest`) |
| [Helm](https://helm.sh/docs/intro/install/) | May need separate installation |
| Docker or Podman | Must be usable without `sudo` |
| curl | Pre-installed on Linux and macOS |
| jq | `brew install jq` / `dnf install jq` / `apt install jq` |
| awk, base64, dd, mktemp | Pre-installed on Linux and macOS |

Verify the key tools:

```bash
kind version && kubectl version --client && helm version && docker version
```

This tool is intended for a disposable local KinD cluster. Do not point it at a
shared or production cluster.

## Platform compatibility

| Platform | Status |
|----------|--------|
| Linux | Runtime-tested (Fedora Linux with Podman) |
| macOS | Designed to work with Docker or Podman and the default macOS Bash, but the complete lifecycle has not yet been runtime-tested on macOS |
| Windows | Native PowerShell/CMD is not supported. Use WSL2 |

## Internet access

`setup.sh` requires internet access to:

- Pull the KinD node image.
- Access the public `prometheus-community` Helm repository.
- Download the `kube-prometheus-stack` chart and container images.

After setup completes, all tool commands (`status`, `fill`, `drain`, `cleanup`)
communicate only with the local KinD cluster through kubectl and localhost
port-forwarding. No internet access is needed for these commands.

## Version reproducibility

The environment versions are currently not pinned:

- The KinD node image is selected by the installed KinD version.
- `setup.sh` installs the latest available `kube-prometheus-stack` Helm chart.

Future chart or KinD releases may change behavior. Pin explicit versions before
relying on this setup for stable CI execution.

## Quick start

```bash
# 1. Create a local KinD cluster and install Prometheus.
./hack/tools/etcd-pressure-simulator/setup.sh

# 2. Confirm the starting state.
./hack/tools/etcd-pressure-simulator/etcd-pressure.sh status

# 3. Create the SET state.
./hack/tools/etcd-pressure-simulator/etcd-pressure.sh fill 95

# E2E test:
# Wait for the alert / DENY behavior and verify the expected system response.

# 4. Create the HOLD state without crossing below 80%.
./hack/tools/etcd-pressure-simulator/etcd-pressure.sh drain 85

# E2E test:
# Verify that the alert / DENY state remains active while usage is between 80% and 95%.

# 5. Cross the RESET threshold.
./hack/tools/etcd-pressure-simulator/etcd-pressure.sh drain 79

# E2E test:
# Verify RESET, keep_firing_for behavior, and the expected allow state.

# 6. Clean the local test environment.
./hack/tools/etcd-pressure-simulator/etcd-pressure.sh cleanup

# 7. Delete the cluster when done.
kind delete cluster --name etcd-shield-test
```

Developers insert their own E2E assertions at the marked points between
commands.

> **Note:** Do not run `hack/e2e_tests.sh` between these commands — it manages
> its own KinD cluster and would delete the pressure-simulator environment.
> Write a dedicated E2E test file for pressure-simulator workflows.

## Expected results

Approximate expected behavior (exact percentages vary by host and timing):

| Command | Expected state |
|---------|---------------|
| Initial status | ~1%--7% usage, 0 pressure CMs |
| `fill 95` | Final usage at or slightly above 95% |
| `drain 85` | Final usage at or below 85%, above the 81% safety floor. The command prints the minimum observed usage. The metric does not cross below 80% |
| `drain 79` | Final usage at or below 79%. This is the intentional RESET-threshold crossing |
| `cleanup` | Usage returns close to the cluster baseline |

### Measured example (one run, Fedora Linux, Podman)

```
Initial: 6.30%
fill 95: 95.08%
drain 85: 84.94%, minimum observed 84.94%
drain 79: 78.92%
cleanup: 1.71%
```

These are measured values from one validation run, not guaranteed exact results.

The full lifecycle takes approximately 35--45 minutes depending on the host,
image cache, network speed, and container runtime. Approximate breakdown:
`setup.sh` ~2--3 min, `fill 95` ~10--12 min, `drain 85` ~15--18 min,
`drain 79` ~7--9 min, `cleanup` ~1--2 min.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_NAME` | `etcd-shield-test` | KinD cluster name |
| `HOLD_SAFETY_FLOOR` | `81.0` | Minimum usage % enforced during HOLD drain |
| `QUOTA_BYTES` | `268435456` | etcd quota in bytes (256 MB). Must match `quota-backend-bytes` in `kind-config.yaml` |
| `PROMETHEUS_LOCAL_PORT` | `19091` | Local port for Prometheus port-forward |

## Troubleshooting

### "Prometheus port-forward failed to become ready"

The Prometheus pod may not be ready yet:

```bash
kubectl get pods -n prometheus
```

Wait for `prometheus-prometheus-kube-prometheus-prometheus-0` to show
`Running`, then retry.

### "etcd_mvcc_db_total_size_in_bytes not found"

Prometheus has not scraped the etcd metrics yet. Wait 30 seconds and retry.
If the problem persists, check that the etcd metrics endpoint is accessible:

```bash
docker exec etcd-shield-test-control-plane curl -s http://localhost:2381/metrics | grep etcd_mvcc_db_total_size
```

Replace `docker` with `podman` if using Podman.

### Cluster already exists

```bash
kind delete cluster --name etcd-shield-test
./hack/tools/etcd-pressure-simulator/setup.sh
```
