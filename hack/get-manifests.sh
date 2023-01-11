#!/bin/sh

DS="daemonset-all.yaml"
case $1 in
evented)
	DS="daemonset-evented.yaml"
	;;
polling)
	DS="daemonset-polling.yaml"
	;;
*)
	DS="daemonset-all.yaml"
	;;
esac

DIRNAME="$(dirname "$(readlink -f "$0")")"
REPOOWNER=${REPOOWNER:-k8stopologyawarewg}
IMAGENAME=${IMAGENAME:-resource-topology-exporter}
IMAGETAG=${IMAGETAG:-latest}
export RTE_CONTAINER_IMAGE=${RTE_CONTAINER_IMAGE:-quay.io/${REPOOWNER}/${IMAGENAME}:${IMAGETAG}}
export RTE_POLL_INTERVAL="${RTE_POLL_INTERVAL:-60s}"
export RTE_VERBOSE="${RTE_VERBOSE:-5}"
export METRICS_PORT="${METRICS_PORT:-2112}"
envsubst < ${DIRNAME}/../manifests/rte/rbac.yaml
envsubst < ${DIRNAME}/../manifests/rte/configmap.yaml
envsubst < ${DIRNAME}/../manifests/rte/${DS}
