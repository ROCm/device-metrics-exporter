TESTRUNNER_IMAGE ?= $(DOCKER_REGISTRY)/$(TESTRUNNER_IMAGE_NAME):$(TESTRUNNER_IMAGE_TAG)
# name used for saving the container images as tar.gz
DOCKER_CONTAINER_IMAGE ?= $(TESTRUNNER_IMAGE_NAME)-$(TESTRUNNER_IMAGE_TAG)
HOURLY_TAG_LABEL ?= latest

ROCM_VERSION ?=
ROCM_TARBALL_URL ?=
RVS_TARBALL_URL ?=

.PHONY: docker
docker:
	@echo "building amd testrunner container with AGFHC=$(AGFHC)"
ifeq ($(AGFHC), 1)
	$(eval DOCKERFILE=$(TOP_DIR)/docker/testrunner/agfhc-dockerfile)
else
	$(eval DOCKERFILE=$(TOP_DIR)/docker/testrunner/Dockerfile)
endif
	@cp $(TOP_DIR)/bin/amd-test-runner .
	@cp $(TOP_DIR)/LICENSE .
	@cp $(TOP_DIR)/docker/install-rocm-tarball.sh .
	@docker build \
		--build-arg BASE_IMAGE=$(TESTRUNNER_RHEL_BASE_IMAGE) \
		$(if $(ROCM_VERSION),--build-arg ROCM_VERSION=$(ROCM_VERSION)) \
		$(if $(ROCM_TARBALL_URL),--build-arg ROCM_TARBALL_URL=$(ROCM_TARBALL_URL)) \
		$(if $(RVS_TARBALL_URL),--build-arg RVS_TARBALL_URL=$(RVS_TARBALL_URL)) \
		-t ${TESTRUNNER_IMAGE} . -f $(DOCKERFILE)
	@rm -f amd-test-runner LICENSE install-rocm-tarball.sh

.PHONY: docker-cicd
docker-cicd:
	@echo "building amd testrunner container with AGFHC=$(AGFHC)"
	@echo "HOURLY_TAG : $(HOURLY_TAG_LABEL)"
ifeq ($(AGFHC), 1)
	$(eval DOCKERFILE=$(TOP_DIR)/docker/testrunner/agfhc-dockerfile)
else
	$(eval DOCKERFILE=$(TOP_DIR)/docker/testrunner/Dockerfile)
endif
	@cp $(TOP_DIR)/bin/amd-test-runner .
	@cp $(TOP_DIR)/LICENSE .
	@cp $(TOP_DIR)/docker/install-rocm-tarball.sh .
	@docker build \
		--build-arg BASE_IMAGE=$(TESTRUNNER_RHEL_BASE_IMAGE) \
		$(if $(ROCM_VERSION),--build-arg ROCM_VERSION=$(ROCM_VERSION)) \
		$(if $(ROCM_TARBALL_URL),--build-arg ROCM_TARBALL_URL=$(ROCM_TARBALL_URL)) \
		$(if $(RVS_TARBALL_URL),--build-arg RVS_TARBALL_URL=$(RVS_TARBALL_URL)) \
		-t ${TESTRUNNER_IMAGE} --label HOURLY_TAG=$(HOURLY_TAG_LABEL) . -f $(DOCKERFILE)
	@rm -f amd-test-runner LICENSE install-rocm-tarball.sh

.PHONY: docker-save
docker-save: ## save the container
	@echo "saving docker image to $(DOCKER_CONTAINER_IMAGE).tar.gz"
	@docker save $(TESTRUNNER_IMAGE) | gzip > $(DOCKER_CONTAINER_IMAGE).tar.gz
