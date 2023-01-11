#!/bin/bash

NAMESPACE="${1:-default}"
DAEMONSET="${2:-resource-topology-exporter-ds}"

FAILED=1

function wait-for-daemonset(){
    retries=24
    while [[ $retries -ge 0 ]];do
        sleep 5
        ready=$(kubectl -n $NAMESPACE get daemonset $DAEMONSET -o jsonpath="{.status.numberReady}")
        required=$(kubectl -n $NAMESPACE get daemonset $DAEMONSET -o jsonpath="{.status.desiredNumberScheduled}")
        if [[ $ready -eq $required ]];then
            echo "${NAMESPACE}/${DAEMONSET} ready"
            FAILED=0
            break
        fi
        ((retries--))
        # debug
	kubectl get ds -n $NAMESPACE $DAEMONSET -o wide
    done
}

echo "waiting for ${NAMESPACE}/${DAEMONSET}"
wait-for-daemonset ${NAMESPACE} ${DAEMONSET}
echo "${NAMESPACE}/${DAEMONSET} wait finished"
kubectl -n $NAMESPACE describe daemonset $DAEMONSET
kubectl -n $NAMESPACE describe pods
exit ${FAILED}
