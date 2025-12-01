OpenShift Image Registry
========================

[![Go Report Card](https://goreportcard.com/badge/github.com/openshift/image-registry)](https://goreportcard.com/report/github.com/openshift/image-registry)
[![GoDoc](https://godoc.org/github.com/openshift/image-registry?status.svg)](https://godoc.org/github.com/openshift/image-registry)
[![Build Status](https://travis-ci.org/openshift/origin.svg?branch=master)](https://travis-ci.org/openshift/origin)
[![Coverage Status](https://coveralls.io/repos/github/openshift/image-registry/badge.svg?branch=master)](https://coveralls.io/github/openshift/image-registry?branch=master)
[![Licensed under Apache License version 2.0](https://img.shields.io/github/license/openshift/image-registry.svg?maxAge=2592000)](https://www.apache.org/licenses/LICENSE-2.0)

***OpenShift Image Registry*** is a tightly integrated with [OpenShift Origin](https://www.openshift.org/) application that lets you distribute Docker images.

Installation and configuration instructions can be found in the
[OpenShift documentation](https://docs.okd.io/latest/registry/architecture-component-imageregistry.html).

**Features:**

* Pull and cache images from remote registries.
* Role-based access control (RBAC).
* Audit log.
* Prometheus metrics.

## Tests

This repository is compatible with the [OpenShift Tests Extension (OTE)](https://github.com/openshift-eng/openshift-tests-extension) framework.

### Building the test binary

```bash
make build
```

### Running test suites and tests

```bash
# Run a specific test suite or test
./dockerregistry-tests-ext run-suite openshift/image-registry/all
./dockerregistry-tests-ext run-test "test-name"

# Run with JUnit output
./dockerregistry-tests-ext run-suite openshift/image-registry/all --junit-path /tmp/junit.xml
```

### Listing available tests and suites

```bash
# List all test suites
./dockerregistry-tests-ext list suites

# List tests in a suite
./dockerregistry-tests-ext list tests --suite=openshift/image-registry/all
```

For more information about the OTE framework, see the [openshift-tests-extension documentation](https://github.com/openshift-eng/openshift-tests-extension).
