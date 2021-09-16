#!/bin/bash
set -eu

DIFF=$( gofmt -s -d cmd pkg test )
if [ -n "${DIFF}" ]; then
	echo "${DIFF}"
	exit 1
fi
exit 0
