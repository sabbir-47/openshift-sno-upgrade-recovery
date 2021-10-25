export GO111MODULE=on
unexport GOPATH

include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
)

CONTAINER_COMMAND = $(shell if [ -x "$(shell which podman)" ];then echo "podman" ; else echo "docker";fi)
IMAGE := $(or ${IMAGE},quay.io/redhat_ztp/openshift-ai-trigger-backup:latest)
GIT_REVISION := $(shell git rev-parse HEAD)
CONTAINER_BUILD_PARAMS = --label git_revision=${GIT_REVISION}

all: build
.PHONY: all

build:
	./hack/build-go.sh
.PHONY: build

build-image:
	$(CONTAINER_COMMAND) build $(CONTAINER_BUILD_PARAMS) -f Dockerfile . -t $(IMAGE)
.PHONY: build-image

push-image:
	$(CONTAINER_COMMAND) push ${IMAGE}
.PHONY: push-image


GO_TEST_PACKAGES :=./pkg/... ./cmd/...
