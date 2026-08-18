EXPORTER_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
EXPORTER_SRIOV_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_SRIOV_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
EXPORTER_AINIC_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_AINIC_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
# name used for saving the container images as tar.gz
DOCKER_CONTAINER_IMAGE ?= $(EXPORTER_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
DOCKER_SRIOV_CONTAINER_IMAGE ?= $(EXPORTER_SRIOV_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
DOCKER_AINIC_CONTAINER_IMAGE ?= $(EXPORTER_AINIC_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
HOURLY_TAG_LABEL ?= latest

ROCM_TARBALL_URL ?=

# gpuagent source model. Threaded from the root Makefile; the
# shared producer (build/gpuagent/) is built once per make invocation and
# build_prep_docker.sh stages its binaries when GPUAGENT_FROM_SOURCE=1.
GPUAGENT_FROM_SOURCE ?= 1
GPUAGENT_BUILD_DIR ?= $(TOP_DIR)/build/gpuagent
# once-downloaded ROCm tarball dir (exported by root Makefile; fallback
# for standalone use). Passed as the `rocm-tarball` build context in tarball mode.
ROCM_TARBALL_DIR ?= $(TOP_DIR)/build/rocm-tarball
AMDSMI_FROM_TARBALL ?= 1

# `docker` builds both the non-SR-IOV and SR-IOV exporter images in one target,
# sharing the single gpuagent producer run across both. Formerly the
# non-SR-IOV image was built by `docker` / `docker-cicd` (two near-duplicate
# targets) and SR-IOV by a separate `docker-sriov` target/job; those are now
# consolidated here. The HOURLY_TAG label (previously docker-cicd-only) is always
# applied. AINIC and azure remain separate targets.
.PHONY: docker
docker:
	@echo "building amd exporter container (non-SR-IOV)"
	@echo "HOURLY_TAG : $(HOURLY_TAG_LABEL)"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	# DOCKER_BUILDKIT=1 required for --build-context / RUN --mount (release runtime
	# stage reads the bind-mounted tarball); overrides a CI DOCKER_BUILDKIT=0.
	# BuildKit's integrated resolver ignores daemon.json insecure-registries for
	# FROM-image metadata; pre-pull the runtime + gostrip bases with the classic
	# client so BuildKit resolves them from the local store instead of over HTTPS.
	docker pull $(RHEL_BASE_MIN_IMAGE)
	$(if $(GPUAGENT_BUILDER_BASE_IMAGE),docker pull $(GPUAGENT_BUILDER_BASE_IMAGE))
	@DOCKER_BUILDKIT=1 docker build --build-arg BASE_IMAGE=$(RHEL_BASE_MIN_IMAGE) \
		--build-arg ROCM_VERSION=$(ROCM_VERSION) \
		$(if $(GPUAGENT_BUILDER_BASE_IMAGE),--build-arg GPUAGENT_BUILDER_BASE_IMAGE=$(GPUAGENT_BUILDER_BASE_IMAGE)) \
		$(if $(GPUAGENT_REPO),--build-arg GPUAGENT_REPO=$(GPUAGENT_REPO)) \
		$(if $(GPUAGENT_COMMIT),--build-arg GPUAGENT_COMMIT=$(GPUAGENT_COMMIT)) \
		--build-arg AMDSMI_FROM_TARBALL=$(if $(AMDSMI_FROM_TARBALL),$(AMDSMI_FROM_TARBALL),0) \
		$(if $(ROCM_TARBALL_URL),--build-arg ROCM_TARBALL_URL=$(ROCM_TARBALL_URL)) \
		$(if $(filter 1,$(AMDSMI_FROM_TARBALL)),--build-context rocm-tarball=$(ROCM_TARBALL_DIR)) \
		-t $(EXPORTER_IMAGE) \
		--label HOURLY_TAG=$(HOURLY_TAG_LABEL) \
		. -f Dockerfile.exporter-release
	@echo "building amd exporter sriov container BASE_IMAGE:$(EXPORTER_SRIOV_BASE_IMAGE), image: $(EXPORTER_SRIOV_IMAGE)"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 SRIOV=1 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(EXPORTER_SRIOV_BASE_IMAGE) -t $(EXPORTER_SRIOV_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.sriov.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-mock
docker-mock:
	@echo "building amd exporter mock container"
	@TOP_DIR=$(TOP_DIR) MOCK=1 OS=RHEL9 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(BUILD_BASE_IMAGE) \
		-t $(EXPORTER_IMAGE) \
		. -f Dockerfile.exporter-mock-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-save
docker-save: ## save the container
ifeq ($(AINIC),1)
	@echo saving docker image to $(DOCKER_AINIC_CONTAINER_IMAGE).tar.gz
	@docker save $(EXPORTER_AINIC_IMAGE) | gzip > $(DOCKER_AINIC_CONTAINER_IMAGE).tar.gz
else
	@echo saving docker image to $(DOCKER_CONTAINER_IMAGE).tar.gz
	@docker save $(EXPORTER_IMAGE) | gzip > $(DOCKER_CONTAINER_IMAGE).tar.gz
endif

.PHONY: azure
azure:
	@echo "building cicd docker for local publish"
	@echo "HOURLY_TAG : $(HOURLY_TAG_LABEL)"
	@TOP_DIR=$(TOP_DIR) OS=AZURE3 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(AZURE_BASE_IMAGE) -t $(EXPORTER_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.azure.linux3.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov
docker-sriov:
	@echo "building amd exporter sriov container BASE_IMAGE:$(EXPORTER_SRIOV_BASE_IMAGE), image: $(EXPORTER_SRIOV_IMAGE)"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 SRIOV=1 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(EXPORTER_SRIOV_BASE_IMAGE) -t $(EXPORTER_SRIOV_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.sriov.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov-ub22
docker-sriov-ub22:
	@echo "building amd exporter sriov container BASE_IMAGE:$(BUILD_BASE_IMAGE), image: $(EXPORTER_SRIOV_IMAGE)"
	# we use the same libs for ubuntu as well
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 SRIOV=1 GPUAGENT_FROM_SOURCE=$(GPUAGENT_FROM_SOURCE) GPUAGENT_BUILD_DIR=$(GPUAGENT_BUILD_DIR) ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(BUILD_BASE_IMAGE) -t $(EXPORTER_SRIOV_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.sriov.ub22.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov-save
docker-sriov-save: ## save the container
	@echo saving docker image to $(DOCKER_SRIOV_CONTAINER_IMAGE).tar.gz
	@docker save $(EXPORTER_SRIOV_IMAGE) | gzip > $(DOCKER_SRIOV_CONTAINER_IMAGE).tar.gz

.PHONY: docker-ainic
docker-ainic:
	@echo "building AMD AINIC exporter container with ainic-versions: $(AINIC_VERSIONS)"
	# AINIC=1 makes build_prep_docker.sh take its early-exit path (exporter-only
	# staging); AINIC does not use gpuagent, so it must not hit the shared source
	# producer staging (build/gpuagent/ is not built for this target).
	@TOP_DIR=$(TOP_DIR) MOCK=$(MOCK) AINIC=1 OS=RHEL9 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(RHEL_BASE_MIN_IMAGE) \
		--build-arg ROCM_VERSION=$(ROCM_VERSION) \
		--build-arg AINIC_VERSIONS=$(AINIC_VERSIONS) \
		$(if $(BOOTSTRAP_VERSION),--build-arg BOOTSTRAP_VERSION=$(BOOTSTRAP_VERSION)) \
		-t $(EXPORTER_AINIC_IMAGE) \
		. -f Dockerfile.ainic.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

