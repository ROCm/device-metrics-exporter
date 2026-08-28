-include dev.env

## Set all the environment variables here
# Docker Registry
DOCKER_REGISTRY ?= docker.io/rocm

# Build Container environment
DOCKER_BUILDER_TAG ?= v1.10
BUILD_BASE_IMAGE ?= ubuntu:22.04
BUILD_CONTAINER ?= $(DOCKER_REGISTRY)/device-metrics-exporter-build:$(DOCKER_BUILDER_TAG)

# In CI environments (e.g. GitHub Actions, cron jobs) there is no TTY, so omit -it flags.
DOCKER_IT_FLAGS := $(if $(CI),,-it)

# Exporter container environment
EXPORTER_IMAGE_TAG ?= latest
EXPORTER_IMAGE_NAME ?= device-metrics-exporter
EXPORTER_SRIOV_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi-minimal:9.8
EXPORTER_SRIOV_IMAGE_NAME ?= device-metrics-exporter-sriov
RHEL_BASE_MIN_IMAGE ?= registry.access.redhat.com/ubi9/ubi-minimal:9.8
AZURE_BASE_IMAGE ?= mcr.microsoft.com/azurelinux/base/core:3.0
EXPORTER_AINIC_IMAGE_NAME ?= device-metrics-exporter-ainic

# helm environment variables
HELM_EXPORTER_IMAGE := $(DOCKER_REGISTRY)/$(EXPORTER_IMAGE_NAME)
HELM_EXPORTER_IMAGE_TAG ?= $(PROJECT_VERSION)

# Test runner container environment
TESTRUNNER_IMAGE_TAG ?= latest
TESTRUNNER_IMAGE_NAME ?= test-runner
TESTRUNNER_RHEL_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi:9.8

# External repo builders
GPUAGENT_BASE_IMAGE ?= ubuntu:22.04
GPUAGENT_BUILDER_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi:9.8
AMDSMI_BASE_IMAGE ?= registry.access.redhat.com/ubi9/ubi:9.6
AMDSMI_BASE_UBUNTU22 ?= ubuntu:22.04
AMDSMI_BASE_UBUNTU24 ?= ubuntu:24.04
AMDSMI_BASE_AZURE ?= mcr.microsoft.com/azurelinux/base/core:3.0
ROCPROFILER_BASE_UBUNTU22 ?= ubuntu:22.04
AMDSMI_BUILDER_IMAGE ?= amdsmi-builder:rhel9
AMDSMI_BUILDER_UB22_IMAGE ?= amdsmi-builder:ub22
AMDSMI_BUILDER_UB24_IMAGE ?= amdsmi-builder:ub24
AMDSMI_BUILDER_AZURE_IMAGE ?= amdsmi-builder:azure
GIMSMI_BUILDER_IMAGE ?= gimsmi-builder:rhel9
GIMSMI_BUILDER_UB22_IMAGE ?= gimsmi-builder:ub22
GIMSMI_BUILDER_UB24_IMAGE ?= gimsmi-builder:ub24
ROCPROFILER_BUILDER_IMAGE ?= rocprofiler-builder:ub22

# export environment variables used across project
export DOCKER_REGISTRY
export BUILD_CONTAINER
export BUILD_BASE_IMAGE
export EXPORTER_IMAGE_NAME
export EXPORTER_SRIOV_BASE_IMAGE
export EXPORTER_SRIOV_IMAGE_NAME
export EXPORTER_IMAGE_TAG
export EXPORTER_AINIC_IMAGE_NAME

# testrunner base images
export TESTRUNNER_IMAGE_NAME
export TESTRUNNER_IMAGE_TAG

# exporter base container images
export TESTRUNNER_RHEL_BASE_IMAGE
export RHEL_BASE_MIN_IMAGE
export AZURE_BASE_IMAGE

# asset builder base images and tags
export AMDSMI_BASE_IMAGE
export AMDSMI_BASE_UBUNTU22
export AMDSMI_BASE_UBUNTU24
export AMDSMI_BASE_AZURE
export GPUAGENT_BUILDER_BASE_IMAGE
export ROCPROFILER_BASE_UBUNTU22

export AMDSMI_BUILDER_IMAGE
export AMDSMI_BUILDER_UB22_IMAGE
export AMDSMI_BUILDER_UB24_IMAGE
export AMDSMI_BUILDER_AZURE_IMAGE
export GPUAGENT_BASE_IMAGE
export ROCPROFILER_BUILDER_IMAGE

TO_GEN := pkg/amdgpu/proto pkg/exporter/proto pkg/amdnic/proto
TO_MOCK := pkg/amdgpu/mock pkg/exporter/scheduler
OUT_DIR := bin
CUR_USER:=$(shell whoami)
CUR_TIME:=$(shell date +%Y-%m-%d_%H.%M.%S)
CONTAINER_NAME:=${CUR_USER}_exporter-bld
CONTAINER_WORKDIR := /usr/src/github.com/ROCm/device-metrics-exporter

TOP_DIR := $(PWD)
# Host-visible repo path for nested `docker run -v` sources under Docker-out-of-Docker; CI sets it to $GITHUB_WORKSPACE.
HOST_TOP_DIR ?= $(TOP_DIR)
GEN_DIR := $(TOP_DIR)/pkg/amdgpu/
MOCK_DIR := ${TOP_DIR}/pkg/amdgpu/mock_gen
HELM_CHARTS_DIR := $(TOP_DIR)/helm-charts
CONFIG_DIR := $(TOP_DIR)/example/
GOINSECURE='github.com, google.golang.org, golang.org'
GOFLAGS ='-buildvcs=false'
GO_BUILD_TAGS ?=
BUILD_DATE ?= $(shell date   +%Y-%m-%dT%H:%M:%S%z)
GIT_COMMIT ?= $(shell git rev-list -1 HEAD --abbrev-commit)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
VERSION ?= $(if $(RELEASE),$(RELEASE),$(GIT_BRANCH))


KUBECONFIG ?= ~/.kube/config

# docs build settings
DOCS_DIR := ${TOP_DIR}/docs
BUILD_DIR := $(DOCS_DIR)/_build
HTML_DIR := $(BUILD_DIR)/html

# library branch to build amdsmi libraries for gpuagent
AMDSMI_REPO   ?= https://github.com/ROCm/rocm-systems.git
# amdsmi source branch bumped release/therock-7.14 -> release/therock-10.0
# to match the ROCM_VERSION tarball bump below. This branch/commit pin only drives the
# from-source amdsmi build (Makefile.compile amdsmi-compile), which is the AMDSMI_FROM_TARBALL=0
# escape hatch; the default path extracts amdsmi from the therock tarball (ROCM_TARBALL_URL).
# TODO: AMDSMI_COMMIT is still the 7.14 SHA and does NOT exist on
# release/therock-10.0. Re-pin to a matching 10.0 commit SHA before using the
# from-source amdsmi escape hatch (AMDSMI_FROM_TARBALL=0).
AMDSMI_BRANCH ?= release/therock-10.0
AMDSMI_COMMIT ?= 53a7a4f3fe6019a551506285f9f2bb86dfddf9b4
AMDSMI_SUBDIR ?= projects/amdsmi
GIMSMI_BRANCH ?= release/9.1.x.K-rc
GIMSMI_COMMIT ?= 9.1.0.K
# gpuagent is cloned (not a submodule) and built inside the exporter docker build.
# COMMIT must be the full 40-char SHA: the in-docker shallow `git fetch --depth 1 <sha>`
# requires a full SHA (abbreviated SHAs are rejected by the remote).
GPUAGENT_REPO ?= https://github.com/ROCm/gpu-agent.git
GPUAGENT_BRANCH ?= main
GPUAGENT_COMMIT ?= 5b2bf2cc2b94eeb98462b8ea0f7f3f1f4fd64f7f

# authoritative ROCm tarball defaults (not overridden in dev.env).
# ROCM_VERSION must match the tarball's version string (extracts to
# /opt/rocm-${ROCM_VERSION}/). Keep URL version in sync; HTTP-200-verify on bump.
ROCM_VERSION ?= 10.0.0
ROCM_TARBALL_URL ?= https://stable.repo.amd.com/rocm/core/tarball/therock-dist-linux-multiarch-10.0.0.tar.gz
# amdsmi commit auto-extracted from the fetched tarball; empty until fetched.
ROCM_COMMIT = $(shell cat "$(ROCM_COMMIT_FILE)" 2>/dev/null)
RVS_TARBALL_URL ?= https://repo.amd.com/rocm/rvs/tarball/amdrocm7-rvs-1.5.122-579-Linux.tar.gz

# download the ~9 GB ROCm tarball ONCE to the host, then bind-mount it
# into every docker build (gpuagent-build stage, release runtime, and the 3
# profiler libbuilder images) via a buildx `--build-context`. The in-Docker
# wget/curl of ROCM_TARBALL_URL is removed — builds read the mounted local file.
# ROCM_TARBALL_DIR is the named build context; ROCM_TARBALL_FILE is the fixed
# filename inside it that Dockerfiles mount.
ROCM_TARBALL_DIR := $(TOP_DIR)/build/rocm-tarball
ROCM_TARBALL_FILE := therock.tar.gz
ROCM_TARBALL_PATH := $(ROCM_TARBALL_DIR)/$(ROCM_TARBALL_FILE)
ROCM_COMMIT_FILE := $(ROCM_TARBALL_DIR)/rocm-commit.txt

# Two-knob source model. Defaults live in dev.env; these `?=`
# fallbacks keep the build robust if dev.env is not sourced. See dev.env for the
# authoritative documentation of each knob.
AMDSMI_FROM_TARBALL ?= 1
GPUAGENT_FROM_SOURCE ?= 1
ROCM_APT_VERSION ?= .apt_7.2.1
AINIC_VERSIONS ?= 1.117.5-a-77,1.117.5-a-147
BOOTSTRAP_VERSION ?= 1.117.5-a-147

# staging dir for amdsmi (libamd_smi.so + amdsmi.h) selectively extracted from the
# ROCm therock tarball by `make amdsmi-from-tarball`; `amdsmi-sync-assets` copies
# it into assets/ (the committed amdsmi the docker build consumes by default)
AMDSMI_TARBALL_STAGE := $(TOP_DIR)/build/amdsmi-from-tarball

export ${GOROOT}
export ${GOPATH}
export ${OUT_DIR}
export ${TOP_DIR}
export ${GOFLAGS}
export ${GOINSECURE}
export ${KUBECONFIG}
export ${AZURE_DOCKER_CONTAINER_IMG}
export ${BUILD_VER_ENV}
export ${AMDSMI_REPO}
export ${AMDSMI_BRANCH}
export ${AMDSMI_COMMIT}
export ${AMDSMI_SUBDIR}
export ${GIMSMI_BRANCH}
export ${GIMSMI_COMMIT}
export GPUAGENT_REPO
export ${GPUAGENT_BRANCH}
export ${GPUAGENT_COMMIT}
export AINIC_VERSIONS
export BOOTSTRAP_VERSION
export ROCM_VERSION
export ROCM_COMMIT
export ROCM_APT_VERSION
export ROCM_TARBALL_URL
export RVS_TARBALL_URL
# export so docker/Makefile (sub-make) can pass --build-context rocm-tarball
export ROCM_TARBALL_DIR
export AMDSMI_FROM_TARBALL
export AMDSMI_FROM_TARBALL
export GPUAGENT_FROM_SOURCE

ASSETS_PATH :=${TOP_DIR}/assets

export ${ASSETS_PATH}
# 22.04 - jammy
# 24.04 - noble
UBUNTU_VERSION ?= jammy
UBUNTU_VERSION_NUMBER = 22.04
UBUNTU_LIBDIR = UBUNTU22
ifeq (${UBUNTU_VERSION}, noble)
UBUNTU_VERSION_NUMBER = 24.04
UBUNTU_LIBDIR = UBUNTU24
endif

# set version and run `make update-version` to all docs
PROJECT_VERSION ?= v1.5.2
HELM_CHARTS_VERSION ?= $(PROJECT_VERSION)
NIC_BUILD ?= 0
ifeq ($(NIC_BUILD),1)
HELM_EXPORTER_IMAGE := $(DOCKER_REGISTRY)/$(EXPORTER_AINIC_IMAGE_NAME)
endif

ifneq (,$(findstring nic-,$(PROJECT_VERSION)))
  # extract v1.0.0 from the nic-v1.0.0 format
  HELM_CHARTS_VERSION := $(subst ",,$(subst nic-,,$(PROJECT_VERSION)))
else ifneq (,$(findstring exporter-,$(PROJECT_VERSION)))
  HELM_CHARTS_VERSION := $(subst ",,$(subst exporter-,,$(PROJECT_VERSION)))
endif

# Derive DEBIAN_VERSION from RELEASE tag
ifneq (,$(findstring exporter,$(RELEASE)))
#remove prefix from main tag
DEBIAN_VERSION := $(shell echo "$(RELEASE)" | sed 's/^exporter-//')
else ifneq (,$(findstring nic,$(RELEASE)))
#parse nic release tag to extract version after "nic-"
DEBIAN_VERSION := $(shell echo "$(RELEASE)" | sed 's/^nic-v//')
else ifneq (,$(findstring v,$(RELEASE)))
#remove prefix for release tag
DEBIAN_VERSION := $(shell echo "$(RELEASE)" | sed 's/^.//')
else
#apt is only released until this version
DEBIAN_VERSION := "1.5.2"
endif

# SR-IOV debian package is versioned independently (pinned to 1.0.0-X), only
# the release label suffix (e.g. "-3" from v1.5.1-3 or exporter-0.0.1-3) is
# carried over from DEBIAN_VERSION.
DEBIAN_SRIOV_VERSION := $(shell echo "$(DEBIAN_VERSION)" | sed -E 's/^[0-9]+\.[0-9]+\.[0-9]+/1.1.0/')

# Remove 'v' from PROJECT_VERSION to get PACKAGE_VERSION
PACKAGE_VERSION := $(subst v,,$(PROJECT_VERSION))

# Split DEBIAN_VERSION on '-' to get RPM version and release labe`l
RPM_BUILD_VERSION := $(word 1,$(subst -, ,$(DEBIAN_VERSION)))
RPM_RELEASE_LABEL_TMP := $(word 2,$(subst -, ,$(DEBIAN_VERSION)))
RPM_RELEASE_LABEL := $(if $(RPM_RELEASE_LABEL_TMP),$(RPM_RELEASE_LABEL_TMP),0)

# Same split, but for the independently-versioned SR-IOV package
RPM_SRIOV_BUILD_VERSION := $(word 1,$(subst -, ,$(DEBIAN_SRIOV_VERSION)))
RPM_SRIOV_RELEASE_LABEL_TMP := $(word 2,$(subst -, ,$(DEBIAN_SRIOV_VERSION)))
RPM_SRIOV_RELEASE_LABEL := $(if $(RPM_SRIOV_RELEASE_LABEL_TMP),$(RPM_SRIOV_RELEASE_LABEL_TMP),0)

REL_IMAGE_TAG := $(PROJECT_VERSION)
HELM_INSTALL_URL := https://github.com/ROCm/device-metrics-exporter/releases/download/${REL_IMAGE_TAG}/device-metrics-exporter-charts-${REL_IMAGE_TAG}\.tgz

export ${DEBIAN_VERSION}
export ${DEBIAN_SRIOV_VERSION}
export ${RPM_BUILD_VERSION}
export ${RPM_SRIOV_BUILD_VERSION}
export ${RPM_SRIOV_RELEASE_LABEL}
export ${RPM_RELEASE_LABEL}

.PHONY: update-version
update-version:
	@echo "Replacing versions with $(PACKAGE_VERSION)..."
	@echo "Helm URL : $(HELM_INSTALL_URL)"
	sed -i -e 's|version = .*|version = ${PACKAGE_VERSION}|' docs/conf.py
ifeq ($(NIC_BUILD),1)
	@NIC_VERSION=$$(echo "$(PROJECT_VERSION)" | sed 's/^nic-v//'); \
	echo "Updating NIC APT repository version to $$NIC_VERSION..."; \
	sed -i -E 's|(https://repo\.radeon\.com/device-metrics-exporter/nic/apt/)[0-9]+\.[0-9]+\.[0-9]+|\1'$$NIC_VERSION'|g' docs/installation/nic-debian-package.md; \
	echo "Updating NIC Docker image tag to $(PROJECT_VERSION)..."; \
	sed -i 's#rocm/device-metrics-exporter:nic-v[0-9]\+\.[0-9]\+\.[0-9]\+#rocm/device-metrics-exporter:$(PROJECT_VERSION)#g' docs/configuration/network-exporter-docker.md
else
	for file in docs/installation/kubernetes-helm.md \
	    helm-charts/values.yaml; do \
	    sed -i -e 's|tag:.*|tag: ${REL_IMAGE_TAG}|' $$file; \
	done
	sed -i -e 's|version="[^"]*"|version="${REL_IMAGE_TAG}"|' docker/Dockerfile.exporter-release
	sed -i -e 's|release="[^"]*"|release="${REL_IMAGE_TAG}"|' docker/Dockerfile.exporter-release
	for file in docs/installation/docker.md \
		docs/installation/singularity.md \
		docs/configuration/configmap.md \
		docs/configuration/docker.md \
		docs/integrations/prometheus-grafana.md; do \
		sed -i 's#v[0-9]\+\.[0-9]\+\.[0-9]\+#${REL_IMAGE_TAG}#g' $$file; \
	done
endif

TO_GEN_TESTRUNNER := pkg/testrunner/proto
GEN_DIR_TESTRUNNER := $(TOP_DIR)/pkg/testrunner/

# ---------------------------------------------------------------------------
# Shared gpuagent producer.
#
# Builds gpuagent + gpuctl + gpuagent_mock + gpuagent_gim ONCE per make
# invocation from source, against the (tarball) amdsmi, by running the
# `gpuagent-build` stage of the release Dockerfile standalone
# (docker build --target gpuagent-build) and extracting the binaries to
# build/gpuagent/. All gpuagent-consuming targets (docker, docker-mock,
# docker-sriov, debpkg, rpmpkg, debpkg-sriov, rpmpkg-sriov) depend on the stamp,
# so `make docker debpkg rpmpkg` in a single invocation builds gpuagent exactly
# once. The stamp file is the "once per make invocation" guarantee.
#
# GPUAGENT_FROM_SOURCE=0 disables the producer; consumers fall back to the
# committed assets/gpuagent_*.bin.gz prebuilt blobs (escape hatch).
#
# NOTE: this deliberately reuses the existing Dockerfile stage rather than
# attempting a host build — the gpuagent from-source build is irreducibly
# containerized (ubi9 toolchain, Go 1.25.11, source pinned at
# /usr/src/github.com/ROCm/gpu-agent, recursive submodules, repo patches,
# ranlib fixes).
# ---------------------------------------------------------------------------
GPUAGENT_BUILD_DIR := $(TOP_DIR)/build/gpuagent
GPUAGENT_BUILD_STAMP := $(GPUAGENT_BUILD_DIR)/.stamp
GPUAGENT_STAGED_IMAGE ?= gpuagent-staged:$(EXPORTER_IMAGE_TAG)
GPUAGENT_STAGE_BIN := /usr/src/github.com/ROCm/gpu-agent/sw/nic/build/x86_64/sim/bin

# When GPUAGENT_FROM_SOURCE=1 (default) gpuagent-consuming targets depend on the
# producer stamp; when =0 the dependency is empty and consumers use assets/.
ifeq ($(GPUAGENT_FROM_SOURCE),1)
GPUAGENT_PRODUCER_DEP := $(GPUAGENT_BUILD_STAMP)
else
GPUAGENT_PRODUCER_DEP :=
endif

# The mock build (docker-mock / e2e) uses the mock gpuagent, which needs no real
# GPU libs, so it defaults to the committed assets/gpuagent_mock.bin.gz blob and
# skips the gpuagent source build AND the ~9 GB ROCm tarball download. Override
# with MOCK_GPUAGENT_FROM_SOURCE=1 to build the mock agent from source instead.
MOCK_GPUAGENT_FROM_SOURCE ?= 0
ifeq ($(MOCK_GPUAGENT_FROM_SOURCE),1)
MOCK_GPUAGENT_PRODUCER_DEP := $(GPUAGENT_BUILD_STAMP)
else
MOCK_GPUAGENT_PRODUCER_DEP :=
endif

# fetch the ROCm tarball once. Idempotent — skips if the file already
# exists (set ROCM_TARBALL_FORCE=1 to re-download). Only needed in tarball mode
# (AMDSMI_FROM_TARBALL=1); ROCM_TARBALL_DEP is empty when =0 so nothing downloads.
ifeq ($(AMDSMI_FROM_TARBALL),1)
ROCM_TARBALL_DEP := $(ROCM_TARBALL_PATH)
else
ROCM_TARBALL_DEP :=
endif

.PHONY: rocm-tarball-fetch
rocm-tarball-fetch: $(ROCM_TARBALL_PATH) $(ROCM_COMMIT_FILE)

$(ROCM_TARBALL_PATH):
	@if [ -n "$(ROCM_TARBALL_FORCE)" ] || [ ! -s "$(ROCM_TARBALL_PATH)" ]; then \
		echo "Downloading ROCm tarball once -> $(ROCM_TARBALL_PATH)"; \
		mkdir -p $(ROCM_TARBALL_DIR); \
		curl -fSL "$(ROCM_TARBALL_URL)" -o $(ROCM_TARBALL_PATH).tmp && \
		mv -f $(ROCM_TARBALL_PATH).tmp $(ROCM_TARBALL_PATH); \
	else \
		echo "ROCm tarball already present: $(ROCM_TARBALL_PATH) (set ROCM_TARBALL_FORCE=1 to re-download)"; \
	fi

$(ROCM_COMMIT_FILE): $(ROCM_TARBALL_PATH)
	@sha=$$(tar xzO -f "$(ROCM_TARBALL_PATH)" ./libexec/amdsmi_cli/_version.py 2>/dev/null \
		| sed -n 's/.*+\([0-9a-f]\{7,40\}\).*/\1/p' | head -1); \
	if [ -n "$$sha" ]; then \
		echo "$$sha" > "$@"; \
		echo "ROCm (amdsmi) commit extracted: $$sha -> $@"; \
	else \
		echo "WARN: could not extract amdsmi commit from tarball"; \
	fi

.PHONY: gpuagent-build
gpuagent-build: $(GPUAGENT_BUILD_STAMP)

$(GPUAGENT_BUILD_STAMP): $(ROCM_TARBALL_DEP)
	@echo "Building shared gpuagent binaries from source (GPUAGENT_FROM_SOURCE=1, AMDSMI_FROM_TARBALL=$(AMDSMI_FROM_TARBALL))"
	# Stage the amdsmi header/lib + gpuagent patches the gpuagent-build stage
	# COPYs from the docker/ build context. In tarball mode the stage overrides
	# these from the tarball, but the COPY lines still need a file present.
	@mkdir -p $(TOP_DIR)/docker/patch-gpuagent
	@cp -vfL $(ASSETS_PATH)/amd_smi_lib/x86_64/RHEL9/lib/amdsmi.h $(TOP_DIR)/docker/amdsmi.h
	@cp -vfL $(ASSETS_PATH)/amd_smi_lib/x86_64/RHEL9/lib/libamd_smi.so.*.*.* $(TOP_DIR)/docker/
	@cp -vfL $(ASSETS_PATH)/amd_smi_lib/x86_64/RHEL9/lib/librocm_sysdeps_*.so* $(TOP_DIR)/docker/ 2>/dev/null || true
	@if ls $(TOP_DIR)/patch/gpuagent/*.patch >/dev/null 2>&1; then \
		cp -vf $(TOP_DIR)/patch/gpuagent/*.patch $(TOP_DIR)/docker/patch-gpuagent/; \
	fi
	# Build only the gpuagent-build stage (produces all four shared binaries).
	# the once-downloaded tarball is bind-mounted via the named build
	# context `rocm-tarball` (only in tarball mode); the stage reads the local file.
	# DOCKER_BUILDKIT=1 is required for --build-context / RUN --mount and overrides
	# any DOCKER_BUILDKIT=0 set by the CI wrapper.
	# BuildKit's integrated (docker:default) resolver ignores daemon.json
	# insecure-registries when resolving FROM-image metadata over HTTP, unlike the
	# classic client. Pre-pull the builder base with the classic client so BuildKit
	# resolves it from the local store instead of over HTTPS.
	docker pull $(GPUAGENT_BUILDER_BASE_IMAGE)
	DOCKER_BUILDKIT=1 docker build --target gpuagent-build \
		$(if $(GPUAGENT_BUILDER_BASE_IMAGE),--build-arg GPUAGENT_BUILDER_BASE_IMAGE=$(GPUAGENT_BUILDER_BASE_IMAGE)) \
		$(if $(GPUAGENT_REPO),--build-arg GPUAGENT_REPO=$(GPUAGENT_REPO)) \
		$(if $(GPUAGENT_COMMIT),--build-arg GPUAGENT_COMMIT=$(GPUAGENT_COMMIT)) \
		--build-arg AMDSMI_FROM_TARBALL=$(AMDSMI_FROM_TARBALL) \
		$(if $(filter 1,$(AMDSMI_FROM_TARBALL)),--build-context rocm-tarball=$(ROCM_TARBALL_DIR)) \
		-t $(GPUAGENT_STAGED_IMAGE) $(TOP_DIR)/docker -f $(TOP_DIR)/docker/Dockerfile.exporter-release
	# Extract the four shared binaries to build/gpuagent/ (once).
	@mkdir -p $(GPUAGENT_BUILD_DIR)
	@cid=$$(docker create $(GPUAGENT_STAGED_IMAGE)); \
	  for b in gpuagent gpuctl gpuagent_mock gpuagent_gim; do \
	    echo "extracting $$b -> $(GPUAGENT_BUILD_DIR)/$$b"; \
	    docker cp $$cid:$(GPUAGENT_STAGE_BIN)/$$b $(GPUAGENT_BUILD_DIR)/$$b; \
	  done; \
	  docker rm -f $$cid
	@touch $@

include Makefile.build
include Makefile.compile
include Makefile.package

##################
# Makefile targets
#
##@ QuickStart
.PHONY: default
default: build-dev-container ## Quick start to build everything from docker shell container
	${MAKE} docker-compile

.PHONY: docker-shell
docker-shell:
	docker run --rm $(DOCKER_IT_FLAGS) --privileged \
		--name ${CONTAINER_NAME} \
		-e "USER_NAME=$(shell whoami)" \
		-e "USER_UID=$(shell id -u)" \
		-e "USER_GID=$(shell id -g)" \
		-e "GIT_COMMIT=${GIT_COMMIT}" \
		-e "GIT_VERSION=${GIT_VERSION}" \
		-e "BUILD_DATE=${BUILD_DATE}" \
		-v $(CURDIR):$(CONTAINER_WORKDIR) \
		-w $(CONTAINER_WORKDIR) \
		$(BUILD_CONTAINER) \
		bash -c "cd $(CONTAINER_WORKDIR) && git config --global --add safe.directory $(CONTAINER_WORKDIR) && bash"

.PHONY: docker-compile
docker-compile:
	docker run --rm $(DOCKER_IT_FLAGS) --privileged \
		--name ${CONTAINER_NAME} \
		-e "USER_NAME=$(shell whoami)" \
		-e "USER_UID=$(shell id -u)" \
		-e "USER_GID=$(shell id -g)" \
		-e "GIT_COMMIT=${GIT_COMMIT}" \
		-e "GIT_VERSION=${GIT_VERSION}" \
		-e "BUILD_DATE=${BUILD_DATE}" \
		-v $(CURDIR):$(CONTAINER_WORKDIR) \
		-w $(CONTAINER_WORKDIR) \
		$(BUILD_CONTAINER) \
		bash -c "cd $(CONTAINER_WORKDIR) && source ~/.bashrc && git config --global --add safe.directory $(CONTAINER_WORKDIR) && make all"

.PHONY: all
all:
	${MAKE} gen amdexporter metricutil amdtestrunner

##@ Configuration Generation
# NOTE: example/config.json is the source of truth for all configurations.
# config-nic.json and config-gpu.json are auto-generated from config.json.
# To update configs, modify config.json and run 'make gen-configs'
.PHONY: gen-configs
gen-configs: ## Generate config-nic.json and config-gpu.json from config.json
	@echo "Generating config-nic.json from config.json"
	@jq '{ServerPort: 5001, CommonConfig: .CommonConfig, NICConfig: .NICConfig}' \
		$(CONFIG_DIR)/config.json > $(CONFIG_DIR)/config-nic.json
	@echo "Generating config-gpu.json from config.json"
	@jq '{ServerPort: .ServerPort, CommonConfig: .CommonConfig, GPUConfig: .GPUConfig}' \
		$(CONFIG_DIR)/config.json > $(CONFIG_DIR)/config-gpu.json
	@echo "Config generation complete"

.PHONY: gen
gen: gen-configs gopkglist gen-test-runner
	@for c in ${TO_GEN}; do printf "\n+++++++++++++++++ Generating $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} GEN_DIR=$(GEN_DIR) || exit 1; done
	@for c in ${TO_MOCK}; do printf "\n+++++++++++++++++ Generating mock $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} MOCK_DIR=$(MOCK_DIR) GEN_DIR=$(GEN_DIR) || exit 1; done

.PHONY: gen-test-runner
gen-test-runner: gopkglist
	@for c in ${TO_GEN_TESTRUNNER}; do printf "\n+++++++++++++++++ Generating $${c} +++++++++++++++++\n"; PATH=$$PATH make -C $${c} GEN_DIR=$(GEN_DIR_TESTRUNNER) || exit 1; done

.PHONY:clean
clean: pkg-clean
	rm -rf bin
	rm -rf docker/obj
	rm -rf docker/*.tgz
	rm -rf docker/*.tar
	rm -rf docker/*.tar.gz
	rm -rf build
	rm -rf helm-charts/*.tgz
	rm -rf helm-charts-k8s

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8)

HELMDOCS = $(shell pwd)/bin/helm-docs
.PHONY: helm-docs
helm-docs: ## Download helm-docs locally if necessary
	$(call go-get-tool,$(HELMDOCS),github.com/norwoodj/helm-docs/cmd/helm-docs@v1.12.0)
	$(HELMDOCS) -c $(shell pwd)/helm-charts/ -g $(shell pwd)/helm-charts -u --ignore-non-descriptions

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
}
endef

EXCLUDE_PATTERN := "libamdsmi|gpuagent.sw|gpuagent.sw.nic|gpuagent.sw.nic.gpuagent"
GO_PKG := $(shell go list ./pkg/... ./tools/... ./test/...  2>/dev/null | grep github.com/ROCm/device-metrics-exporter | egrep -v ${EXCLUDE_PATTERN})

GOFILES_NO_VENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./libamdsmi/*" -not -path "./gpuagent/*")
.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	@if [ `gofmt -l $(GOFILES_NO_VENDOR) | wc -l` -ne 0 ]; then \
		echo There are some malformed files, please make sure to run \'make fmt\'; \
		gofmt -l $(GOFILES_NO_VENDOR); \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run -v $(shell find . \( -path "./vendor" -o -path "./libamdsmi" -o -path "./gpuagent" -o -path "./tools/metricutil" \) \
		-prune -o -type f -name '*.go' -print | xargs -n1 dirname | sort -u)

.PHONY: fmt
fmt:## Run go fmt against code.
	go fmt $(GO_PKG)

.PHONY: vet
vet: ## Run go vet against code.
	$(info +++ govet sources)
	go vet -source $(GO_PKG)

.PHONY: gopkglist
gopkglist:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install go.uber.org/mock/mockgen@v0.6.0
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	go install golang.org/x/tools/cmd/goimports@v0.40.0
	go install github.com/alta/protopatch/cmd/protoc-gen-go-patch@v0.5.3

amdexporter: metricsclient amdgpuhealth
	@echo "building amd metrics exporter"
	CGO_ENABLED=0 go build -C cmd/exporter \
		$(if $(GO_BUILD_TAGS),-tags $(GO_BUILD_TAGS)) \
		-ldflags "-X main.Version=${VERSION} \
		          -X main.GitCommit=${GIT_COMMIT} \
		          -X main.BuildDate=${BUILD_DATE} \
		          -X main.ROCmCommit=${ROCM_COMMIT}" \
		-o $(CURDIR)/bin/amd-metrics-exporter

amdtestrunner:
	@echo "building amd test runner"
	CGO_ENABLED=0 go build  -C cmd/testrunner -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" -o $(CURDIR)/bin/amd-test-runner

metricutil:
	@echo "building metrics util"
	CGO_ENABLED=0 go build -C tools/metricutil -o $(CURDIR)/bin/metricutil

metricsclient:
	@echo "building metrics client"
	CGO_ENABLED=0 go build -C tools/metricsclient -o $(CURDIR)/bin/metricsclient

amdgpuhealth:
	@echo "building amd gpu health util"
	CGO_ENABLED=0 go build -C tools/amd-gpu-health -o $(CURDIR)/bin/amdgpuhealth

# `docker` and `docker-sriov` are composed from single-purpose helper targets via
# prerequisites, so no build step is written twice. Shared prerequisites (gen,
# amdexporter, and the gpuagent producer $(GPUAGENT_PRODUCER_DEP)) are built ONCE
# per make run even though several helpers depend on them — that is the point of
# using make dependencies here.
#
#   make docker        = both exporter images (non-SR-IOV + SR-IOV) + all deb + all rpm
#   make docker-sriov  = SR-IOV image + SR-IOV deb + SR-IOV rpm only (a subset)
#
# The former `docker-cicd` target is folded into the image helper (HOURLY_TAG
# always applied). AINIC (docker-ainic) and azure (docker-azure) stay separate.

# both exporter images (non-SR-IOV + SR-IOV), saved as tar.gz
.PHONY: docker-image
docker-image: gen amdexporter $(GPUAGENT_PRODUCER_DEP)
	${MAKE} -C docker TOP_DIR=$(CURDIR) HOURLY_TAG_LABEL=$(HOURLY_TAG_LABEL) AMDSMI_FROM_TARBALL=$(AMDSMI_FROM_TARBALL) GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ROCM_TARBALL_URL=$(ROCM_TARBALL_URL)
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR)
	${MAKE} -C docker docker-sriov-save TOP_DIR=$(CURDIR)

# SR-IOV exporter image only, saved as tar.gz
.PHONY: docker-image-sriov
docker-image-sriov: gen amdexporter $(GPUAGENT_PRODUCER_DEP)
	echo "Building docker for sriov driver rhel9"
	${MAKE} -C docker docker-sriov TOP_DIR=$(CURDIR) GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR)
	${MAKE} -C docker docker-sriov-save TOP_DIR=$(CURDIR)

# all deb (Ubuntu 22 + 24) + rpm (RHEL9), non-SR-IOV + SR-IOV. One libcopy per OS
# builds both variants (no repeated libcopy). ub24 is a recursive
# UBUNTU_VERSION=noble sub-make because UBUNTU_VERSION is process-global.
# EXPORTER_PREBUILT=1: amdexporter is already built once as a prereq above, so the
# package sub-makes skip their own `${MAKE} amdexporter` rebuild (the binary is a
# CGO-static amd64 build, identical across ub22/ub24/rhel9). Standalone
# `make debpkg` / `make rpmpkg` (no EXPORTER_PREBUILT) still rebuild fresh.
.PHONY: docker-pkgs
docker-pkgs: gen amdexporter $(GPUAGENT_PRODUCER_DEP)
	${MAKE} EXPORTER_PREBUILT=1 libcopy-assets-RHEL9 rpmpkg rpmpkg-sriov
	${MAKE} EXPORTER_PREBUILT=1 libcopy-assets-UBUNTU22 debpkg debpkg-sriov
	${MAKE} EXPORTER_PREBUILT=1 UBUNTU_VERSION=noble libcopy-assets-UBUNTU24 debpkg debpkg-sriov

# SR-IOV-only deb (Ubuntu 22 + 24) + rpm (RHEL9); one libcopy per OS.
.PHONY: docker-pkgs-sriov
docker-pkgs-sriov: gen amdexporter $(GPUAGENT_PRODUCER_DEP)
	${MAKE} EXPORTER_PREBUILT=1 libcopy-assets-RHEL9 rpmpkg-sriov
	${MAKE} EXPORTER_PREBUILT=1 libcopy-assets-UBUNTU22 debpkg-sriov
	${MAKE} EXPORTER_PREBUILT=1 UBUNTU_VERSION=noble libcopy-assets-UBUNTU24 debpkg-sriov

.PHONY: docker
docker: docker-image docker-pkgs

.PHONY: docker-sriov
docker-sriov: docker-image-sriov docker-pkgs-sriov

.PHONY: docker-ainic
docker-ainic: gen amdexporter
	echo "Building docker for ainic driver rhel9"
	${MAKE} -C docker docker-ainic TOP_DIR=$(CURDIR)
	${MAKE} -C docker docker-save  TOP_DIR=$(CURDIR) AINIC=1

# for development we use ubuntu based. Builds gpuagent_gim + gpuctl from the
# shared source producer (GPUAGENT_FROM_SOURCE=1 default), like docker-sriov.
.PHONY: docker-sriov-ub22
docker-sriov-ub22: gen amdexporter $(GPUAGENT_PRODUCER_DEP)
	echo "Building docker for sriov driver ub22"
	${MAKE} -C docker docker-sriov-ub22 TOP_DIR=$(CURDIR) GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR)
	${MAKE} -C docker docker-sriov-save TOP_DIR=$(CURDIR)

.PHONY: docker-mock
docker-mock: gen $(MOCK_GPUAGENT_PRODUCER_DEP)
	GO_BUILD_TAGS=mock ${MAKE} amdexporter
	${MAKE} mock-rocpctl
	${MAKE} -C docker TOP_DIR=$(CURDIR) EXPORTER_IMAGE_NAME=$(EXPORTER_IMAGE_NAME)-mock GPUAGENT_FROM_SOURCE=$(MOCK_GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) docker-mock
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR) EXPORTER_IMAGE_NAME=$(EXPORTER_IMAGE_NAME)-mock

.PHONY: docker-test-runner
docker-test-runner: gen-test-runner amdtestrunner
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) docker \
		ROCM_VERSION=$(TESTRUNNER_ROCM_VERSION) \
		ROCM_TARBALL_URL=$(TESTRUNNER_ROCM_TARBALL_URL) \
		RVS_TARBALL_URL=$(RVS_TARBALL_URL)

# AGFHC=1 pins ROCm to the rocm7 tarball (AGFHC bundle is rocm7-only, see dev.env);
# the default (RVS-only) build uses the repo-wide ROCM_VERSION.
TESTRUNNER_ROCM_VERSION = $(if $(filter 1,$(AGFHC)),$(AGFHC_ROCM_VERSION),$(ROCM_VERSION))
TESTRUNNER_ROCM_TARBALL_URL = $(if $(filter 1,$(AGFHC)),$(AGFHC_ROCM_TARBALL_URL),$(ROCM_TARBALL_URL))

.PHONY: docker-test-runner-cicd
docker-test-runner-cicd: gen-test-runner amdtestrunner
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) docker-cicd \
		ROCM_VERSION=$(TESTRUNNER_ROCM_VERSION) \
		ROCM_TARBALL_URL=$(TESTRUNNER_ROCM_TARBALL_URL) \
		RVS_TARBALL_URL=$(RVS_TARBALL_URL)
	${MAKE} -C docker/testrunner TOP_DIR=$(CURDIR) docker-save

# Pinned to the prebuilt blob path (GPUAGENT_FROM_SOURCE=0) so it stages
# gpuagent/gpuctl from assets/ without the shared source producer.
.PHONY: docker-azure
docker-azure: gen amdexporter
	${MAKE} -C docker azure TOP_DIR=$(CURDIR) GPUAGENT_FROM_SOURCE=0
	${MAKE} -C docker docker-save TOP_DIR=$(CURDIR) DOCKER_CONTAINER_IMAGE=${EXPORTER_IMAGE_NAME}-${EXPORTER_IMAGE_TAG}-azure

.PHONY:checks
checks: gen vet lint

.PHONY: docker-publish
docker-publish:
	${MAKE} -C docker docker-publish TOP_DIR=$(CURDIR)

.PHONY: unit-test
unit-test:
	PATH=$$PATH LOGDIR=$(TOP_DIR)/ go test -v -cover -mod=vendor ./pkg/...

mod:
	@echo "ignoring submodules gpuagent and libamdsmi"
	@touch ${TOP_DIR}/gpuagent/go.mod
	@touch ${TOP_DIR}/libamdsmi/go.mod
	@echo "setting up go mod packages"
	@go mod tidy
	@go mod edit -go=1.25.10
	#CVE-2024-24790 - amd-metrics-exporter
	@go mod edit -replace golang.org/x/net@v0.29.0=golang.org/x/net@v0.36.0
	#CVE-2026-33186
	@go mod edit -replace google.golang.org/grpc@v1.72.1=google.golang.org/grpc@v1.79.3
	#CVE-2025-30204 - amd-test-runner
	@go mod edit -replace github.com/golang-jwt/jwt/v5@v5.2.1=github.com/golang-jwt/jwt/v5@v5.2.2
	#CVE GHSA-fv92-fjc5-jj9h - amdgpuhealth
	@go mod edit -replace github.com/go-viper/mapstructure/v2@v2.2.1=github.com/go-viper/mapstructure/v2@v2.4.0
	@go mod vendor
	@rm ${TOP_DIR}/gpuagent/go.mod
	@rm ${TOP_DIR}/libamdsmi/go.mod

.PHONY: docs clean-docs dep-docs
dep-docs:
	pip install -r $(DOCS_DIR)/sphinx/requirements.txt

docs: dep-docs
	sphinx-build -b html $(DOCS_DIR) $(HTML_DIR)
	@echo "Docs built at $(HTML_DIR)/index.html"

clean-docs:
	rm -rf $(BUILD_DIR)

DOCS_MARKDOWNLINTCONFIG ?= docs/.markdownlint-cli2.yaml
DOCS_MD_GLOB ?= "**/*.md"
DOCS_SPELLCHECK_CONFIG ?= .spellcheck.yaml

.PHONY: docs-lint-markdown
docs-lint-markdown:
	markdownlint-cli2 $(DOCS_MD_GLOB) --config $(DOCS_MARKDOWNLINTCONFIG)

.PHONY: docs-lint-spelling
docs-lint-spelling:
	pyspelling -c $(DOCS_SPELLCHECK_CONFIG)

.PHONY: docs-lint
docs-lint: ## Run docs Markdown lint + spelling (full ROCm-style docs lint).
	${MAKE} docs-lint-markdown
	${MAKE} docs-lint-spelling

.PHONY: doc-audit
doc-audit: ## Verify metricslist.md matches metrics-support-matrix.yaml (CI gate).
	@bash tools/scripts/doc-audit.sh

.PHONY: base-image
base-image:
	${MAKE} -C tools/base-image

copyrights:
	GOFLAGS=-mod=mod go run tools/build/copyright/main.go && ${MAKE} fmt && ./tools/build/check-local-files.sh

# target to update remote submodule repo for amdsmi and gpuagent
.PHONY: update-submodules
update-submodules:
	git submodule update --remote --recursive

.PHONY: e2e-test
e2e-test:
	$(MAKE) -C test/e2e

.PHONY: e2e
e2e:
	$(MAKE) docker-mock
	$(MAKE) e2e-test

# Real-GIM SR-IOV e2e. Builds the sriov image (real libgim_amd_smi.so) and
# runs the hardware-bound TestSRIOVRealGIM on a live GIM host. The test skips
# unless SRIOV_EXPORTER_IMAGE points at the built image.
.PHONY: e2e-sriov
e2e-sriov:
	$(MAKE) docker-sriov
	SRIOV_EXPORTER_IMAGE=$(DOCKER_REGISTRY)/$(EXPORTER_SRIOV_IMAGE_NAME):$(EXPORTER_IMAGE_TAG) \
		go test ./test/e2e/ -run TestSRIOVRealGIM -v -count=1

.PHOHY: k8s-e2e
k8s-e2e:
	TOP_DIR=$(CURDIR) $(MAKE) -C test/k8s-e2e

.PHONY: helm-lint
helm-lint:
	@echo "Project Version is $(PROJECT_VERSION)"
	@echo "RELEASE tag is $(RELEASE)"
	#copy default config
ifeq ($(NIC_BUILD),1)
	jq 'del(.ServerPort, .NICConfig.ExtraPodLabels)' $(CONFIG_DIR)/config-nic.json > $(HELM_CHARTS_DIR)/config.json
else
	jq 'del(.ServerPort, .GPUConfig.ExtraPodLabels)' $(CONFIG_DIR)/config-gpu.json > $(HELM_CHARTS_DIR)/config.json
endif
	cd $(HELM_CHARTS_DIR); helm lint .

# cicd target to build helm chart - requires PROJECT_VERSION to be set
.PHONY: helm
helm:
	@rm -rf helm-charts-k8s
	${MAKE} helm-build

.PHONY: helm-build
helm-build: helm-lint
	@rm -rf helm-charts/nic-device-metrics-exporter*
	@rm -rf helm-charts/device-metrics-exporter*
ifeq ($(NIC_BUILD),1)
	@echo "\n+++++++++++++++++ Building NIC monitoring helm chart ++++++++++++++++\n"
else
	@echo "\n+++++++++++++++++ Building GPU monitoring helm chart ++++++++++++++++\n"
endif
	# updating project version in helm Chart.yaml
	@yq eval -i '.appVersion = "$(HELM_CHARTS_VERSION)"' helm-charts/Chart.yaml
	@yq eval -i '.version = "$(HELM_CHARTS_VERSION)"' helm-charts/Chart.yaml
	# set exporter image repo and tag
	@yq eval -i '.image.repository = "$(HELM_EXPORTER_IMAGE)"' helm-charts/values.yaml
	@yq eval -i '.image.tag = "$(HELM_EXPORTER_IMAGE_TAG)"' helm-charts/values.yaml

# update monitoring flags in values.yaml based on RELEASE tag
ifeq ($(NIC_BUILD),1)
	@echo "NIC build — enabling NIC monitoring";
	@yq eval -i '.name = "nic-device-metrics-exporter-charts"' helm-charts/Chart.yaml;
	@yq eval -i '.monitor.resources.nic = true | .monitor.resources.gpu = false' helm-charts/values.yaml;
	@yq eval -i '.hostNetwork = true' helm-charts/values.yaml;
	@yq eval -i '.service.ClusterIP.port = 5001' helm-charts/values.yaml;
	@yq eval -i '.service.NodePort.port = 5001' helm-charts/values.yaml;
	@yq eval -i '.service.NodePort.nodePort = 32501' helm-charts/values.yaml;
else
	@echo "Standard build — enabling GPU monitoring";
	@yq eval -i '.name = "device-metrics-exporter-charts"' helm-charts/Chart.yaml;
	@yq eval -i '.monitor.resources.nic = false | .monitor.resources.gpu = true' helm-charts/values.yaml;
	@yq eval -i '.hostNetwork = false' helm-charts/values.yaml;
	@yq eval -i '.service.ClusterIP.port = 5000' helm-charts/values.yaml;
	@yq eval -i '.service.NodePort.port = 5000' helm-charts/values.yaml;
	@yq eval -i '.service.NodePort.nodePort = 32500' helm-charts/values.yaml;
endif

	${MAKE} helm-docs
	@mkdir -p helm-charts-k8s
	helm package helm-charts/ --destination ./helm-charts

ifeq ($(NIC_BUILD),1)
	cp -vf helm-charts/nic-device-metrics-exporter-charts-$(HELM_CHARTS_VERSION).tgz helm-charts/nic-device-metrics-exporter-charts.tgz
	helm template nic-device-metrics-exporter helm-charts/nic-device-metrics-exporter-charts.tgz -n kube-amd-network -f helm-charts/values.yaml > helm-charts/manifests.yaml
	kubectl kustomize helm-charts/ > /dev/null || { echo "Error: kubectl kustomize failed"; rm -rf helm-charts/manifests.yaml; exit 1; }
	rm -rf helm-charts/manifests.yaml
	cp -vf helm-charts/nic-device-metrics-exporter-charts-$(HELM_CHARTS_VERSION).tgz helm-charts-k8s/nic-device-metrics-exporter-helm-k8s-${PROJECT_VERSION}.tgz
else
	cp -vf helm-charts/device-metrics-exporter-charts-$(HELM_CHARTS_VERSION).tgz helm-charts/device-metrics-exporter-charts.tgz
	helm template device-metrics-exporter helm-charts/device-metrics-exporter-charts.tgz -n kube-amd-gpu -f helm-charts/values.yaml > helm-charts/manifests.yaml
	kubectl kustomize helm-charts/ > /dev/null || { echo "Error: kubectl kustomize failed"; rm -rf helm-charts/manifests.yaml; exit 1; }
	rm -rf helm-charts/manifests.yaml
	cp -vf helm-charts/device-metrics-exporter-charts-$(HELM_CHARTS_VERSION).tgz helm-charts-k8s/device-metrics-exporter-helm-k8s-${PROJECT_VERSION}.tgz
endif

.PHONY: slurm-sim
slurm-sim:
	${MAKE} -C pkg/exporter/scheduler/slurmsim TOP_DIR=$(CURDIR)

# create development build container only if there is changes done on
# tools/base-image/Dockerfile
.PHONY: build-dev-container
build-dev-container:
	${MAKE} -C tools/base-image all INSECURE_REGISTRY=$(INSECURE_REGISTRY)

.PHONY: amdsmi-compile-all
amdsmi-compile-all:
	${MAKE} amdsmi-compile-ub24
	${MAKE} amdsmi-compile-ub22
	${MAKE} amdsmi-compile-rhel
	#${MAKE} amdsmi-compile-azure

.PHONY: gimsmi-compile-all
gimsmi-compile-all:
	${MAKE} gimsmi-compile-ub24
	${MAKE} gimsmi-compile-ub22
	${MAKE} gimsmi-compile-rhel

# build all components
.PHONY: build-all
build-all: 
	${MAKE} amdsmi-compile-all
	${MAKE} gimsmi-compile-all

.PHONY: mock-rocpctl
mock-rocpctl:
	${MAKE} -C tools/mock-rocpctl BIN_PATH=$(CURDIR)/bin
