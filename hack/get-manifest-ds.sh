#!/bin/sh

DIRNAME="$(dirname "$(readlink -f "$0")")"
DEFAULT_IMAGE="quay.io/k8stopologyawarewg/resource-topology-exporter:latest"
REPOOWNER=${REPOOWNER:-k8stopologyawarewg}
IMAGENAME=${IMAGENAME:-resource-topology-exporter}
IMAGETAG=${IMAGETAG:-latest}
export RTE_CONTAINER_IMAGE=${RTE_CONTAINER_IMAGE:-quay.io/${REPOOWNER}/${IMAGENAME}:${IMAGETAG}}
envsubst < ${DIRNAME}/../manifests/resource-topology-exporter-ds.yaml
