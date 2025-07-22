TESTRUNNER_IMAGE ?= $(DOCKER_REGISTRY)/$(TESTRUNNER_IMAGE_NAME):$(TESTRUNNER_IMAGE_TAG)
# name used for saving the container images as tar.gz
DOCKER_CONTAINER_IMAGE ?= $(TESTRUNNER_IMAGE_NAME)-$(TESTRUNNER_IMAGE_TAG)
HOURLY_TAG_LABEL ?= latest

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
	@docker build --build-arg BASE_IMAGE=$(TEST_RUNNER_RHEL_BASE_IMAGE) -t ${TESTRUNNER_IMAGE} . -f $(TOP_DIR)/docker/testrunner/Dockerfile
	@rm -f $(TOP_DIR)/docker/testrunner/amd-test-runner
	@rm -f $(TOP_DIR)/docker/testrunner/LICENSE

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
	@docker build --build-arg BASE_IMAGE=$(TEST_RUNNER_RHEL_BASE_IMAGE) -t ${TESTRUNNER_IMAGE} . -f $(TOP_DIR)/docker/testrunner/Dockerfile
	@rm -f $(TOP_DIR)/docker/testrunner/amd-test-runner
	@rm -f $(TOP_DIR)/docker/testrunner/LICENSE

.PHONY: docker-save
docker-save: ## save the container
	@echo "saving docker image to $(DOCKER_CONTAINER_IMAGE).tar.gz"
	@docker save $(TESTRUNNER_IMAGE) | gzip > $(DOCKER_CONTAINER_IMAGE).tar.gz
