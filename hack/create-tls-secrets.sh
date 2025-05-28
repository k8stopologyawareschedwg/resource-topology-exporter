#!/bin/sh
set -xe

BASEDIR="$(dirname "$(readlink -f "$0")")"

kubectl create secret tls rte-tls-secret --cert=${BASEDIR}/../config/ci/tls/server.crt --key=${BASEDIR}/../config/ci/tls/server.key
kubectl create configmap rte-tls-ca.crt --from-file ca.crt=${BASEDIR}/../config/ci/tls/ca.crt

