#!/bin/bash
#
# Copyright 2025 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ROOT=$(realpath "$(dirname "${0}")/..")
OUTDIR="${ROOT}/out"
CLUSTER_NAME="etcd-shield-test"
IMAGE_BUILDER=${IMAGE_BUILDER:-podman}

set -o pipefail

mkdir -p "${OUTDIR}"

function start_cluster() {
    if [[ "$(kind get clusters -q | grep ${CLUSTER_NAME})" -eq ${CLUSTER_NAME} ]]; then
        # we don't know the current cluster state, so restart the cluster
        kind delete cluster -n ${CLUSTER_NAME}
    fi
    kind create cluster -n ${CLUSTER_NAME}
    kind get kubeconfig -n ${CLUSTER_NAME} > "${OUTDIR}/kubeconfig"

    export KUBECONFIG="${ROOT}/out/kubeconfig"
}

function deploy_prometheus() {
    NAMESPACE="prometheus"
    TMPDIR="$(mktemp -d)"
    LATEST=$(curl -s https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/latest | jq -cr .tag_name)
    curl -s "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/refs/tags/${LATEST}/kustomization.yaml" > "$TMPDIR/kustomization.yaml"
    curl -s "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/refs/tags/${LATEST}/bundle.yaml" > "$TMPDIR/bundle.yaml"
    kubectl create namespace ${NAMESPACE}
    (cd "${TMPDIR}" && kustomize edit set namespace ${NAMESPACE}) && kubectl create -k "${TMPDIR}"

    # TODO: finish deploying prometheus.  This only deploys an instance of
    # prometheus operator, not of prometheus itself.
}

function deploy_cert_manager() {
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.0/cert-manager.yaml
    kubectl wait \
        deployments \
        --for=condition=Available \
        -n cert-manager \
        --timeout=5m \
        -l app.kubernetes.io/instance=cert-manager
}

function build_and_deploy_etcd_shield() {
    pushd "${ROOT}" || exit
    local IMG=etcd-shield:latest
    make build-image "IMG=${IMG}" "IMAGE_BUILDER=${IMAGE_BUILDER}"
    "${IMAGE_BUILDER}" save "${IMG}" | kind load image-archive /dev/stdin -n "${CLUSTER_NAME}"

    pushd "${OUTDIR}" || exit
        # remove kustomization manifest if it exists
        [[ -e "./kustomization.yaml" ]] && rm ./kustomization.yaml
        kustomize init
        kustomize edit add resource ../acceptance/config/
        [[ "${IMAGE_BUILDER}" == "podman" ]] && IMG="localhost/${IMG}"
        kustomize edit set image "etcd-shield=${IMG}"
        kustomize build | kubectl apply -f -
    popd || exit

    popd || exit
}

start_cluster || exit 1
deploy_cert_manager || exit 1
deploy_prometheus || exit 1
build_and_deploy_etcd_shield || exit 1
