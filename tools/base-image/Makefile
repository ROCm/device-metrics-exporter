all:
	@if [ -n "$(INSECURE_REGISTRY)" ]; then \
	echo "building dev container with insecure registry" ;\
	docker build --build-arg USER=$(shell id -un) --build-arg GROUP=$(shell id -gn) \
		--build-arg BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE) \
		--build-arg INSECURE_REGISTRY=$(INSECURE_REGISTRY) \
		--build-arg UID=$(shell id -u) --build-arg GID=$(shell id -g) -t ${BUILD_CONTAINER} . ;\
	else \
	echo "building dev container without insecure registry" ;\
	docker build --build-arg USER=$(shell id -un) --build-arg GROUP=$(shell id -gn) \
		--build-arg BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE) \
		--build-arg UID=$(shell id -u) --build-arg GID=$(shell id -g) -t ${BUILD_CONTAINER} . ;\
	fi
	@echo "dev container build complete : ${BUILD_CONTAINER}"
