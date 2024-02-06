#!/bin/bash

BASEDIR="$(dirname "$(readlink -f "$0")")"
ARGS="$@"

set -x

${BASEDIR}/../_out/resource-topology-exporter \
	--kubeconfig=${KUBECONFIG} \
	--podresources-socket=fake:///${BASEDIR}/../test/data/fake \
       	--kubelet-config-file=config/examples/kubeletconf.yaml \
	--podreadiness=false \
	${ARGS}
