-include dev.env

## Set all the environment variables here
# Docker Registry
DOCKER_REGISTRY ?= docker.io/rocm

# Build Container environment
DOCKER_BUILDER_TAG ?= v1.1
BUILD_BASE_IMAGE ?= ubuntu:22.04
BUILD_CONTAINER ?= $(DOCKER_REGISTRY)/device-metrics-exporter-build:$(DOCKER_BUILDER_TAG)

# Exporter container environment
EXPORTER_IMAGE_TAG ?= latest
EXPORTER_IMAGE_NAME ?= device-metrics-exporter
RHEL_BASE_MIN_IMAGE ?= registry.access.redhat.com/ubi9/ubi-minimal:9.4
AZURE_BASE_IMAGE ?= mcr.microsoft.com/azurelinux/base/core:3.0

# Test runner container environment
TESTRUNNER_IMAGE_TAG ?= latest
TESTRUNNER_IMAGE_NAME ?= test-runner
RHEL_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi:9.4

# export environment variables used across project
export DOCKER_REGISTRY
export BUILD_CONTAINER
export BUILD_BASE_IMAGE
export EXPORTER_IMAGE_NAME
export EXPORTER_IMAGE_TAG
export TESTRUNNER_IMAGE_NAME
export TESTRUNNER_IMAGE_TAG
export RHEL_BASE_IMAGE
export RHEL_BASE_MIN_IMAGE
export AZURE_BASE_IMAGE

TO_GEN := pkg/amdgpu/proto pkg/exporter/proto
TO_MOCK := pkg/amdgpu/mock
OUT_DIR := bin
CUR_USER:=$(shell whoami)
CUR_TIME:=$(shell date +%Y-%m-%d_%H.%M.%S)
CONTAINER_NAME:=${CUR_USER}_exporter-bld
CONTAINER_WORKDIR := /usr/src/github.com/ROCm/device-metrics-exporter

TOP_DIR := $(PWD)
GEN_DIR := $(TOP_DIR)/pkg/amdgpu/
MOCK_DIR := ${TOP_DIR}/pkg/amdgpu/mock_gen
HELM_CHARTS_DIR := $(TOP_DIR)/helm-charts
CONFIG_DIR := $(TOP_DIR)/example/
GOINSECURE='github.com, google.golang.org, golang.org'
GOFLAGS ='-buildvcs=false'
BUILD_DATE ?= $(shell date   +%Y-%m-%dT%H:%M:%S%z)
GIT_COMMIT ?= $(shell git rev-list -1 HEAD --abbrev-commit)
VERSION ?=$(RELEASE)
KUBECONFIG ?= ~/.kube/config

export ${GOROOT}
export ${GOPATH}
export ${OUT_DIR}
export ${TOP_DIR}
export ${GOFLAGS}
export ${GOINSECURE}
export ${KUBECONFIG}
export ${AZURE_DOCKER_CONTAINER_IMG}
export ${BUILD_VER_ENV}

ASSETS_PATH :=${TOP_DIR}/assets
# 22.04 - jammy
# 24.04 - noble
UBUNTU_VERSION ?= jammy
UBUNTU_VERSION_NUMBER = 22.04
ifeq (${UBUNTU_VERSION}, noble)
UBUNTU_VERSION_NUMBER = 24.04
endif

DEBIAN_VERSION := "1.2.0"

DEBIAN_CONTROL = ${TOP_DIR}/debian/DEBIAN/control
BUILD_VER_ENV = ${DEBIAN_VERSION}~$(UBUNTU_VERSION_NUMBER)
PATCH_LIBS := ${ASSETS_PATH}/patch/ubuntu/${UBUNTU_VERSION}
GPUAGENT_LIBS := ${ASSETS_PATH}/amd_smi_lib/x86_64/${UBUNTU_VERSION}/lib
THIRDPARTY_LIBS := ${ASSETS_PATH}/thirdparty/x86_64-linux-gnu/${UBUNTU_VERSION}/lib/
PKG_PATH := ${TOP_DIR}/debian/usr/local/bin
PKG_LIB_PATH := ${TOP_DIR}/debian/usr/local/metrics/
LUA_PROTO := ${TOP_DIR}/pkg/amdgpu/proto/luaplugin.proto
PKG_LUA_PATH := ${TOP_DIR}/debian/usr/local/etc/metrics/slurm

TO_GEN_TESTRUNNER := pkg/testrunner/proto
GEN_DIR_TESTRUNNER := $(TOP_DIR)/pkg/testrunner/

##################
# Makefile targets
#
##@ QuickStart
.PHONY: default
default: build-dev-container ## Quick start to build everything from docker shell container
	${MAKE} docker-compile

.PHONY: docker-shell
docker-shell:
	docker run --rm -it --privileged \
		--name ${CONTAINER_NAME} \
		-e "USER_NAME=$(shell whoami)" \
		-e "USER_UID=$(shell id -u)" \
		-e "USER_GID=$(shell id -g)" \
		-e "GIT_COMMIT=${GIT_COMMIT}" \
		-e "GIT_VERSION=${GIT_VERSION}" \
		-e "BUILD_DATE=${BUILD_DATE}" \
		-v $(CURDIR):$(CONTAINER_WORKDIR) \
		-v $(HOME)/.ssh:/home/$(shell whoami)/.ssh \
		-w $(CONTAINER_WORKDIR) \
		$(BUILD_CONTAINER) \
		bash -c "cd $(CONTAINER_WORKDIR) && git config --global --add safe.directory $(CONTAINER_WORKDIR) && bash"

.PHONY: docker-compile
docker-compile:
	docker run --rm -it --privileged \
		--name ${CONTAINER_NAME} \
		-e "USER_NAME=$(shell whoami)" \
		-e "USER_UID=$(shell id -u)" \
		-e "USER_GID=$(shell id -g)" \
		-e "GIT_COMMIT=${GIT_COMMIT}" \
		-e "GIT_VERSION=${GIT_VERSION}" \
		-e "BUILD_DATE=${BUILD_DATE}" \
		-v $(CURDIR):$(CONTAINER_WORKDIR) \
		-v $(HOME)/.ssh:/home/$(shell whoami)/.ssh \
		-w $(CONTAINER_WORKDIR) \
		$(BUILD_CONTAINER) \
		bash -c "cd $(CONTAINER_WORKDIR) && source ~/.bashrc && git config --global --add safe.directory $(CONTAINER_WORKDIR) && make all"

.PHONY: all
all:
	${MAKE} gen amdexporter metricutil amdtestrunner

.PHONY: gen
gen: gopkglist gen-test-runner
	@for c in ${TO_GEN}; do printf "\n+++++++++++++++++ Generating $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} GEN_DIR=$(GEN_DIR) || exit 1; done
	@for c in ${TO_MOCK}; do printf "\n+++++++++++++++++ Generating mock $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} MOCK_DIR=$(MOCK_DIR) GEN_DIR=$(GEN_DIR) || exit 1; done

.PHONY: gen-test-runner
gen-test-runner: gopkglist
	@for c in ${TO_GEN_TESTRUNNER}; do printf "\n+++++++++++++++++ Generating $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} GEN_DIR=$(GEN_DIR_TESTRUNNER) || exit 1; done

.PHONY: pkg pkg-clean

pkg-clean:
	rm -rf ${TOP_DIR}/bin/*.deb


pkg: pkg-clean
	${MAKE} gen amdexporter-lite metricsclient
	@echo "Building debian for $(BUILD_VER_ENV)"
	#copy precompiled libs
	mkdir -p ${PKG_LIB_PATH}
	cp -rvf ${GPUAGENT_LIBS}/ ${PKG_LIB_PATH}
	cp -rvf ${THIRDPARTY_LIBS}/ ${PKG_LIB_PATH}
	#override patch files
	cp -vf ${PATCH_LIBS}/libamd_smi.so.24.7.60300 ${PKG_LIB_PATH}/lib/libamd_smi.so.24.7.60301
	#copy and strip files
	mkdir -p ${PKG_PATH}
	tar -xf ${ASSETS_PATH}/gpuagent_static.bin.gz -C ${PKG_PATH}/
	chmod +x ${PKG_PATH}/gpuagent
	ls -alsh ${PKG_PATH}/gpuagent
	#strip prebuilt binaries
	strip ${PKG_PATH}/gpuagent
	ls -alsh ${PKG_PATH}/gpuagent
	cd ${PKG_PATH} && strip ${PKG_PATH}/gpuagent
	cp -vf ${LUA_PROTO} ${PKG_LUA_PATH}/plugin.proto
	cp -vf ${ASSETS_PATH}/gpuctl.gobin ${PKG_PATH}/gpuctl
	cp -vf $(CURDIR)/bin/amd-metrics-exporter ${PKG_PATH}/
	cp -vf $(CURDIR)/bin/metricsclient ${PKG_PATH}/
	cd ${TOP_DIR}
	sed -i "s/BUILD_VER_ENV/$(BUILD_VER_ENV)/g" $(DEBIAN_CONTROL)
	dpkg-deb -Zxz --build debian ${TOP_DIR}/bin
	#remove copied files
	rm -rf ${PKG_LIB_PATH}
	rm -rf ${PKG_LUA_PATH}/plugin.proto
	# revert the dynamic version set file
	git checkout $(DEBIAN_CONTROL)
	# rename for internal build
	mv -vf ${TOP_DIR}/bin/amdgpu-exporter_*~${UBUNTU_VERSION_NUMBER}_amd64.deb ${TOP_DIR}/bin/amdgpu-exporter_${UBUNTU_VERSION_NUMBER}_amd64.deb

.PHONY:clean
clean: pkg-clean
	rm -rf pkg/amdgpu/gen
	rm -rf bin
	rm -rf docker/obj
	rm -rf docker/*.tgz
	rm -rf docker/*.tar
	rm -rf docker/*.tar.gz
	rm -rf ${PKG_PATH}

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.1)

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
}
endef

GOFILES_NO_VENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")
.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	@if [ `gofmt -l $(GOFILES_NO_VENDOR) | wc -l` -ne 0 ]; then \
		echo There are some malformed files, please make sure to run \'make fmt\'; \
		gofmt -l $(GOFILES_NO_VENDOR); \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run -v --timeout 5m0s

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...


.PHONY: gopkglist
gopkglist:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install go.uber.org/mock/mockgen@v0.5.0
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.1
	go install golang.org/x/tools/cmd/goimports@latest

amdexporter-lite:
	@echo "building lite version of metrics exporter"
	go build -C cmd/exporter -ldflags "-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE} -X main.Publish=${DISABLE_DEBUG} " -o $(CURDIR)/bin/amd-metrics-exporter


amdexporter: metricsclient
	@echo "building amd metrics exporter"
	CGO_ENABLED=0 go build  -C cmd/exporter -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE} -X main.Publish=${DISABLE_DEBUG}" -o $(CURDIR)/bin/amd-metrics-exporter

amdtestrunner:
	@echo "building amd test runner"
	CGO_ENABLED=0 go build  -C cmd/testrunner -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" -o $(CURDIR)/bin/amd-test-runner

metricutil:
	@echo "building metrics util"
	CGO_ENABLED=0 go build -C tools/metricutil -o $(CURDIR)/bin/metricutil

metricsclient:
	@echo "building metrics client"
	CGO_ENABLED=0 go build -C tools/metricsclient -o $(CURDIR)/bin/metricsclient

.PHONY: docker-cicd
docker-cicd: gen amdexporter
	echo "Building cicd docker for publish"
	${MAKE} -C docker docker-cicd TOP_DIR=$(CURDIR)
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR)

.PHONY: docker
docker: gen amdexporter
	${MAKE} -C docker TOP_DIR=$(CURDIR)
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR)

.PHONY: docker-mock
docker-mock: gen amdexporter
	${MAKE} -C docker TOP_DIR=$(CURDIR) MOCK=1 EXPORTER_IMAGE_NAME=$(EXPORTER_IMAGE_NAME)-mock
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR) EXPORTER_IMAGE_NAME=$(EXPORTER_IMAGE_NAME)-mock

.PHONY: docker-test-runner
docker-test-runner: gen-test-runner amdtestrunner
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) INTERNAL_TESTRUNNER_BUILD=$(INTERNAL_TESTRUNNER_BUILD) docker

.PHOHY: docker-test-runner-cicd
docker-test-runner-cicd: gen-test-runner amdtestrunner
	echo "Building test runner cicd docker for publish"
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) docker-cicd
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) docker-save

.PHONY: docker-azure
docker-azure: gen amdexporter
	${MAKE} -C docker azure TOP_DIR=$(CURDIR)
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR) DOCKER_CONTAINER_IMAGE=${EXPORTER_IMAGE_NAME}-${EXPORTER_IMAGE_TAG}-azure

.PHONY:checks
checks: gen vet

.PHONY: docker-publish
docker-publish:
	${MAKE} -C docker docker-publish TOP_DIR=$(CURDIR)

.PHONY: unit-test
unit-test:
	PATH=$$PATH LOGDIR=$(TOP_DIR)/ go test -v -cover -mod=vendor ./pkg/...

loadgpu:
	sudo modprobe amdgpu

mod:
	@echo "setting up go mod packages"
	@go mod tidy
	@go mod vendor

.PHONY: base-image
base-image:
	${MAKE} -C tools/base-image

copyrights:
	GOFLAGS=-mod=mod go run tools/build/copyright/main.go && ${MAKE} fmt && ./tools/build/check-local-files.sh

.PHONY: e2e-test
e2e-test:
	$(MAKE) -C test/e2e

.PHONY: e2e
e2e:
	$(MAKE) docker-mock
	$(MAKE) e2e-test

.PHOHY: k8s-e2e
k8s-e2e:
	TOP_DIR=$(CURDIR) $(MAKE) -C test/k8s-e2e

.PHONY: helm-lint
helm-lint:
	jq 'del(.ServerPort, .GPUConfig.CustomLabels)' $(CONFIG_DIR)/config.json > $(HELM_CHARTS_DIR)/config.json
	cd $(HELM_CHARTS_DIR); helm lint

.PHONY: helm-build
helm-build: helm-lint
	helm package helm-charts/ --destination ./helm-charts

.PHONY: slurm-sim
slurm-sim:
	${MAKE} -C pkg/exporter/scheduler/slurmsim TOP_DIR=$(CURDIR)

# create development build container only if there is changes done on
# tools/base-image/Dockerfile
.PHONY: build-dev-container
build-dev-container:
	${MAKE} -C tools/base-image all INSECURE_REGISTRY=$(INSECURE_REGISTRY)
