# `etcd-shield`

Denies Tekton `PipelineRuns` from getting created on a cluster if `etcd` is getting too full.

# Dependencies

This requires Tekton's `CustomResourceDefinitions` to be installed, specifically the `PipelineRun` CRD.

# Architecture

There are three main components to `etcd-shield`:
- Prometheus queries
- Metrics

## Prometheus queries

We need to talk to a Prometheus instance that has `etcd` metrics in order to determine if its safe to
let more `PipelineRun` resources into the cluster.

### State tracking

The states for ingress denial is analogous to a [JK flip-flip], where we have separate
clocked `set` and `reset` signals.  The `set` signal will be defined to be `etcd` usage going above
a specified `etcd` usage threshold, while the `reset` signal will be `etcd` usage dropping beneath some other
specified threshold.

If we set the `reset` threshold to be below the `set` threshold, we can give the cluster a little
breathing room between when we decide to allow `PipelineRuns` and when we need to start denying them
again.  This will help prevent alert spam and allow users to potentially get more than a single
`PipelineRun` into the cluster before we have to stop allowing new ones in.

## Metrics

We also expose some Prometheus metrics on `localhost:9100/metrics`.  This allows us to hook into
things like `AlertManager`.

Metrics exposed:
- `etcd_shield_allow`: `0` if new `PipelineRun` resources are not allowed, `1` if they are.

## Webhooks

Webhooks are implemented using Kubebuilder. They listen for creation events
for `PipelineRuns` and decide whether to allow or deny resources based on the value we
stored in the `ConfigMap` above.

We've separated webhooks out from Prometheus queries for a few reasons:

- We're anticipating the webhooks to be called frequently, so checking Prometheus on every admission
request could cause a lot of load on Prometheus.
- We can scale responding to admission requests independently from running Prometheus queries.

[JK flip-flop]: https://en.wikipedia.org/wiki/Flip-flop_(electronics)#JK_flip-flop
