#!/bin/bash

# This script provides constants for the Golang binary build process

readonly OS_BUILD_ENV_GOLANG="${OS_BUILD_ENV_GOLANG:-1.8}"
readonly OS_BUILD_ENV_IMAGE="${OS_BUILD_ENV_IMAGE:-openshift/origin-release:golang-${OS_BUILD_ENV_GOLANG}}"

readonly OS_OUTPUT_BASEPATH="${OS_OUTPUT_BASEPATH:-_output}"
readonly OS_BASE_OUTPUT="${OS_ROOT}/${OS_OUTPUT_BASEPATH}"
readonly OS_OUTPUT_SCRIPTPATH="${OS_OUTPUT_SCRIPTPATH:-"${OS_BASE_OUTPUT}/scripts"}"

readonly OS_OUTPUT_SUBPATH="${OS_OUTPUT_SUBPATH:-${OS_OUTPUT_BASEPATH}/local}"
readonly OS_OUTPUT="${OS_ROOT}/${OS_OUTPUT_SUBPATH}"
readonly OS_OUTPUT_RELEASEPATH="${OS_OUTPUT}/releases"
readonly OS_OUTPUT_RPMPATH="${OS_OUTPUT_RELEASEPATH}/rpms"
readonly OS_OUTPUT_BINPATH="${OS_OUTPUT}/bin"
readonly OS_OUTPUT_PKGDIR="${OS_OUTPUT}/pkgdir"

readonly OS_GO_PACKAGE=github.com/openshift/image-registry

readonly OS_IMAGE_COMPILE_TARGETS_LINUX=(
  cmd/dockerregistry
)

readonly OS_IMAGE_COMPILE_BINARIES=("${OS_IMAGE_COMPILE_TARGETS_LINUX[@]##*/}")

readonly OS_BINARY_RELEASE_SERVER_LINUX=(
  './*'
)
readonly OS_BINARY_RELEASE_CLIENT_EXTRA=(
  ${OS_ROOT}/README.md
  ${OS_ROOT}/LICENSE
)
