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

set -e -o pipefail

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
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update
    helm install prometheus prometheus-community/kube-prometheus-stack \
      --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
      --set prometheus.prometheusSpec.ruleSelectorNilUsesHelmValues=false \
      --namespace "${NAMESPACE}" --create-namespace
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

function install_required_tekton_crds() {
  curl -Ls https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml | \
    yq 'select(.metadata.name=="pipelineruns.tekton.dev")' | \
    kubectl apply -f -
}

function build_etcd_shield() {
    pushd "${ROOT}" || exit
    local IMG=etcd-shield:latest
    make build-image "IMG=${IMG}" "IMAGE_BUILDER=${IMAGE_BUILDER}"
    "${IMAGE_BUILDER}" save "${IMG}" | kind load image-archive /dev/stdin -n "${CLUSTER_NAME}"
    popd || exit
}

function deploy_etcd_shield() {
    local IMG=etcd-shield:latest
    pushd "${OUTDIR}" || exit
        # remove kustomization manifest if it exists
        [[ -e "./kustomization.yaml" ]] && rm ./kustomization.yaml
        kustomize init
        kustomize edit add resource ../acceptance/config/
        [[ "${IMAGE_BUILDER}" == "podman" ]] && IMG="localhost/${IMG}"
        kustomize edit set image "etcd-shield=${IMG}"
        kustomize build | kubectl apply -f -
    popd || exit
}

start_cluster || exit 1
install_required_tekton_crds || exit 1
deploy_cert_manager || exit 1
deploy_prometheus || exit 1
build_etcd_shield || exit 1
deploy_etcd_shield || exit 1
