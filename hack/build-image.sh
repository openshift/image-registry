#!/bin/bash

set -e

OS_ROOT=$(dirname ${BASH_SOURCE})/..

# Register function to be called on EXIT to remove generated binary.
function cleanup {
  rm "${OS_ROOT}/images/dockerregistry/bin/dockerregistry"
}
trap cleanup EXIT

cp -v ${OS_ROOT}/_output/bin/dockerregistry "${OS_ROOT}/images/dockerregistry/bin/dockerregistry"
docker build -t openshift/origin-docker-registry:latest ${OS_ROOT}/images/dockerregistry