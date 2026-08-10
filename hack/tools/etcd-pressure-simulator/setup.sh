#!/bin/bash
#
# Sets up a KinD cluster and Prometheus for use with etcd-pressure-simulator.
#
# This creates the environment needed for the gradual pressure simulator.
# It does not deploy etcd-shield or run any E2E tests.
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${0}")" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-etcd-shield-test}"

if kind get clusters -q 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists. Delete it first with:"
    echo "  kind delete cluster -n ${CLUSTER_NAME}"
    exit 1
fi

echo "Creating KinD cluster '${CLUSTER_NAME}'..."
kind create cluster \
    -n "${CLUSTER_NAME}" \
    --config "${SCRIPT_DIR}/kind-config.yaml"

echo ""
echo "Installing Prometheus with etcd scraping..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/kube-prometheus-stack \
    -f "${SCRIPT_DIR}/prometheus-values.yaml" \
    -n prometheus --create-namespace

echo ""
echo "Waiting for Prometheus operator to be ready..."
kubectl wait --for=condition=Ready pod \
    -l app=kube-prometheus-stack-operator \
    -n prometheus \
    --timeout=300s

echo "Waiting for Prometheus server to be ready..."
kubectl wait --for=condition=Ready pod \
    -l app.kubernetes.io/name=prometheus \
    -n prometheus \
    --timeout=300s 2>/dev/null || \
kubectl wait --for=condition=Ready pod \
    -l operator.prometheus.io/name=prometheus-kube-prometheus-prometheus \
    -n prometheus \
    --timeout=300s

echo ""
echo "Setup complete."
echo ""
echo "Usage:"
echo "  ${SCRIPT_DIR}/etcd-pressure.sh status"
echo "  ${SCRIPT_DIR}/etcd-pressure.sh fill 95"
echo "  ${SCRIPT_DIR}/etcd-pressure.sh drain 85"
echo "  ${SCRIPT_DIR}/etcd-pressure.sh drain 79"
echo "  ${SCRIPT_DIR}/etcd-pressure.sh cleanup"
