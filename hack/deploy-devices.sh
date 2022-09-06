#!/bin/bash

DIRNAME="$(dirname "$(readlink -f "$0")")"

kubectl create -f ${DIRNAME}/../manifests/sample-devices/test-devicepluginA-config.yaml
kubectl create -f ${DIRNAME}/../manifests/sample-devices/test-devicepluginA-ds.yaml
