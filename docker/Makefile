EXPORTER_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
EXPORTER_SRIOV_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_SRIOV_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
EXPORTER_AINIC_IMAGE ?= $(DOCKER_REGISTRY)/$(EXPORTER_AINIC_IMAGE_NAME):$(EXPORTER_IMAGE_TAG)
# name used for saving the container images as tar.gz
DOCKER_CONTAINER_IMAGE ?= $(EXPORTER_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
DOCKER_SRIOV_CONTAINER_IMAGE ?= $(EXPORTER_SRIOV_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
DOCKER_AINIC_CONTAINER_IMAGE ?= $(EXPORTER_AINIC_IMAGE_NAME)-$(EXPORTER_IMAGE_TAG)
HOURLY_TAG_LABEL ?= latest

.PHONY: docker
docker:
	@echo "building amd exporter container"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(RHEL_BASE_MIN_IMAGE) \
		--build-arg ROCM_VERSION=$(ROCM_VERSION) \
		-t $(EXPORTER_IMAGE) \
		. -f Dockerfile.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-mock
docker-mock:
	@echo "building amd exporter mock container"
	@TOP_DIR=$(TOP_DIR) MOCK=1 OS=RHEL9 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(BUILD_BASE_IMAGE) \
		-t $(EXPORTER_IMAGE) \
		. -f Dockerfile.exporter-mock-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-cicd
docker-cicd:
	@echo "building cicd docker for local publish"
	@echo "HOURLY_TAG : $(HOURLY_TAG_LABEL)"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(RHEL_BASE_MIN_IMAGE) \
		--build-arg ROCM_VERSION=$(ROCM_VERSION) \
		-t $(EXPORTER_IMAGE) \
		--label HOURLY_TAG=$(HOURLY_TAG_LABEL) \
		. -f Dockerfile.exporter-release
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
	@TOP_DIR=$(TOP_DIR) OS=AZURE3 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(AZURE_BASE_IMAGE) -t $(EXPORTER_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.azure.linux3.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov
docker-sriov:
	@echo "building amd exporter sriov container BASE_IMAGE:$(EXPORTER_SRIOV_BASE_IMAGE), image: $(EXPORTER_SRIOV_IMAGE)"
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 SRIOV=1 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(EXPORTER_SRIOV_BASE_IMAGE) -t $(EXPORTER_SRIOV_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.sriov.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov-ub22
docker-sriov-ub22:
	@echo "building amd exporter sriov container BASE_IMAGE:$(BUILD_BASE_IMAGE), image: $(EXPORTER_SRIOV_IMAGE)"
	# we use the same libs for ubuntu as well
	@TOP_DIR=$(TOP_DIR) OS=RHEL9 SRIOV=1 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(BUILD_BASE_IMAGE) -t $(EXPORTER_SRIOV_IMAGE) --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f Dockerfile.sriov.ub22.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

.PHONY: docker-sriov-save
docker-sriov-save: ## save the container
	@echo saving docker image to $(DOCKER_SRIOV_CONTAINER_IMAGE).tar.gz
	@docker save $(EXPORTER_SRIOV_IMAGE) | gzip > $(DOCKER_SRIOV_CONTAINER_IMAGE).tar.gz

.PHONY: docker-ainic
docker-ainic:
	@echo "building AMD AINIC exporter container with ainic-version: $(AINIC_VERSION)"
	@TOP_DIR=$(TOP_DIR) MOCK=$(MOCK) OS=RHEL9 ./build_prep_docker.sh
	@docker build --build-arg BASE_IMAGE=$(RHEL_BASE_MIN_IMAGE) \
		--build-arg ROCM_VERSION=$(ROCM_VERSION) \
		--build-arg AINIC_VERSION=$(AINIC_VERSION) \
		-t $(EXPORTER_AINIC_IMAGE) \
		. -f Dockerfile.ainic.exporter-release
	@TOP_DIR=$(TOP_DIR) ./build_post_docker.sh

