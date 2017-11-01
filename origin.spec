#debuginfo not supported with Go
%global debug_package %{nil}
# modifying the Go binaries breaks the DWARF debugging
%global __os_install_post %{_rpmconfigdir}/brp-compress

%global gopath      %{_datadir}/gocode
%global import_path github.com/openshift/image-registry

%global golang_version 1.8.1
# %commit and %os_git_vars are intended to be set by tito custom builders provided
# in the .tito/lib directory. The values in this spec file will not be kept up to date.
%{!?commit:
%global commit ea924726fd4950d86b7a501217f16cc83809b745
}
%global shortcommit %(c=%{commit}; echo ${c:0:7})
# os_git_vars needed to run hack scripts during rpm builds
%{!?os_git_vars:
%global os_git_vars OS_GIT_COMMIT=ea924726 OS_GIT_VERSION=v3.7.0-0.0.0.0.0.rc.0.4.4ba92a6.1.49f5dbf.0.c01409b.0.4fe3b99.1.ed1b7fe+ea92472-2 OS_GIT_MAJOR=3 OS_GIT_MINOR=7+ OS_GIT_TREE_STATE=clean
}

%if 0%{?skip_build}
%global do_build 0
%else
%global do_build 1
%endif

%if 0%{?fedora} || 0%{?epel}
%global need_redistributable_set 0
%else
# Due to library availability, redistributable builds only work on x86_64
%ifarch x86_64
%global need_redistributable_set 1
%else
%global need_redistributable_set 0
%endif
%endif
%{!?make_redistributable: %global make_redistributable %{need_redistributable_set}}

%if "%{dist}" == ".el7aos"
%global package_name atomic-openshift
%global product_name Atomic OpenShift
%else
%global package_name origin
%global product_name Origin
%endif

Name:           %{package_name}
# Version is not kept up to date and is intended to be set by tito custom
# builders provided in the .tito/lib directory of this project
Version:        3.7.0
Release:        0.0.0.0.0.0.rc.0.4.4ba92a6.1.49f5dbf.0.c01409b.0.4fe3b99.1.ed1b7fe.2.ea92472
Summary:        Docker Registry v2 for %{product_name}
License:        ASL 2.0
URL:            https://%{import_path}

# If go_arches not defined fall through to implicit golang archs
%if 0%{?go_arches:1}
ExclusiveArch:  %{go_arches}
%else
ExclusiveArch:  x86_64 aarch64 ppc64le s390x
%endif

Source0:        https://%{import_path}/archive/%{commit}/%{name}-%{version}.tar.gz
BuildRequires:  bsdtar
BuildRequires:  golang >= %{golang_version}
BuildRequires:  rsync

#
# The following Bundled Provides entries are populated automatically by the
# OpenShift Origin tito custom builder found here:
#   https://github.com/openshift/origin/blob/master/.tito/lib/origin/builder/
#
# These are defined as per:
# https://fedoraproject.org/wiki/Packaging:Guidelines#Bundling_and_Duplication_of_system_libraries
#
### AUTO-BUNDLED-GEN-ENTRY-POINT

%description
OpenShift is a distribution of Kubernetes optimized for enterprise application
development and deployment. OpenShift adds developer and operational centric
tools on top of Kubernetes to enable rapid application development, easy
deployment and scaling, and long-term lifecycle maintenance for small and large
teams and applications. It provides a secure and multi-tenant configuration for
Kubernetes allowing you to safely host many different applications and workloads
on a unified cluster.

%prep
%setup -q

%build
%if 0%{do_build}
%if 0%{make_redistributable}
# Create Binaries for all supported arches
%{os_git_vars} hack/build-go.sh cmd/dockerregistry
%else
# Create Binaries only for building arch
%ifarch x86_64
  BUILD_PLATFORM="linux/amd64"
%endif
%ifarch %{ix86}
  BUILD_PLATFORM="linux/386"
%endif
%ifarch ppc64le
  BUILD_PLATFORM="linux/ppc64le"
%endif
%ifarch %{arm} aarch64
  BUILD_PLATFORM="linux/arm64"
%endif
%ifarch s390x
  BUILD_PLATFORM="linux/s390x"
%endif
OS_ONLY_BUILD_PLATFORMS="${BUILD_PLATFORM}" %{os_git_vars} hack/build-go.sh cmd/dockerregistry
%endif
%endif

%install

PLATFORM="$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
install -d %{buildroot}%{_bindir}

# Install linux components
for bin in dockerregistry
do
  echo "+++ INSTALLING ${bin}"
  install -p -m 755 _output/local/bin/${PLATFORM}/${bin} %{buildroot}%{_bindir}/${bin}
done

%files
%doc README.md
%license LICENSE

%files
%{_bindir}/dockerregistry

%changelog
* Tue Oct 31 2017 Clayton Coleman <ccoleman@redhat.com> - 0-0.0.1
- Update to latest upstream.