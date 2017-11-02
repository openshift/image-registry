all: build
.PHONY: all

build:
	go build -o _output/bin/dockerregistry github.com/openshift/image-registry/cmd/dockerregistry
.PHONY: build

build-image: build
	hack/build-image.sh
.PHONY: build-image

verify: build
	go test github.com/openshift/image-registry/pkg/...
.PHONY: verify

clean:
	rm -rf _output
.PHONY: clean
