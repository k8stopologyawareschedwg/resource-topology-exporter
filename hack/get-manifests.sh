#!/bin/sh

DIRNAME="$(dirname "$(readlink -f "$0")")"
DEFAULT_IMAGE="quay.io/k8stopologyawarewg/resource-topology-exporter:latest"
REPOOWNER=${REPOOWNER:-k8stopologyawarewg}
IMAGENAME=${IMAGENAME:-resource-topology-exporter}
IMAGETAG=${IMAGETAG:-latest}
export RTE_CONTAINER_IMAGE=${RTE_CONTAINER_IMAGE:-quay.io/${REPOOWNER}/${IMAGENAME}:${IMAGETAG}}
export RTE_POLL_INTERVAL="${RTE_POLL_INTERVAL:-60s}"
export RTE_VERBOSE="${RTE_VERBOSE:-5}"
export RTE_METRICS_MODE="${RTE_METRICS_MODE:-disabled}"
export RTE_METRICS_CLI_AUTH="${RTE_METRICS_CLI_AUTH:-true}"
export METRICS_PORT="${METRICS_PORT:-2112}"
export METRICS_ADDRESS="${METRICS_ADDRESS:-127.0.0.1}"
envsubst < ${DIRNAME}/../manifests/resource-topology-exporter.yaml
