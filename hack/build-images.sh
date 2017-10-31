#!/bin/bash

# This script builds all images locally except the base and release images,
# which are handled by hack/build-base-images.sh.

# NOTE:  you only need to run this script if your code changes are part of
# any images OpenShift runs internally such as origin-sti-builder, origin-docker-builder,
# origin-deployer, etc.
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

function cleanup() {
    return_code=$?
    os::util::describe_return_code "${return_code}"
    exit "${return_code}"
}
trap "cleanup" EXIT

os::util::ensure::gopath_binary_exists imagebuilder
# image builds require RPMs to have been built
os::build::release::check_for_rpms
# OS_RELEASE_COMMIT is required by image-build
os::build::archive::detect_local_release_tars $(os::build::host_platform_friendly)

# we need to mount RPMs into the container builds for installation
OS_BUILD_IMAGE_ARGS="${OS_BUILD_IMAGE_ARGS:-} -mount ${OS_OUTPUT_RPMPATH}/:/srv/origin-local-release/"

# Create link to file if the FS supports hardlinks, otherwise copy the file
function ln_or_cp {
	local src_file=$1
	local dst_dir=$2
	if os::build::archive::internal::is_hardlink_supported "${dst_dir}" ; then
		ln -f "${src_file}" "${dst_dir}"
	else
		cp -pf "${src_file}" "${dst_dir}"
	fi
}

# determine the correct tag prefix
tag_prefix="${OS_IMAGE_PREFIX:-"openshift/origin"}"

for i in `jobs -p`; do wait $i; done

# images that depend on "${tag_prefix}-base"
( os::build::image "${tag_prefix}-docker-registry"       images/dockerregistry ) &

for i in `jobs -p`; do wait $i; done


