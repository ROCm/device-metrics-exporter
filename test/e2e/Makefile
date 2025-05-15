.DEFAULT: all
.PHONY: all lint

TEST_ARGS ?= ""

all:
	go clean -testcache && DOCKER_REGISTRY=$(DOCKER_REGISTRY) EXPORTER_IMAGE_NAME=$(EXPORTER_IMAGE_NAME)-mock EXPORTER_IMAGE_TAG=$(EXPORTER_IMAGE_TAG) go test -test.timeout=30m -clean-after-test -v $(TEST_ARGS);

lint:
	@go fmt ./...
	@goimports -w ./
	@go vet ./...
