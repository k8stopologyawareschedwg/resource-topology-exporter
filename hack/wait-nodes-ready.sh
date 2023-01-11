#!/bin/sh
KUBECTL="${KUBECTL:-kubectl}"

exec ${KUBECTL} wait --for=condition=Ready nodes --all --timeout=600s
