.PHONY: bld_proto
TOGEN := $(shell ls *.proto)
bld_proto:
	@echo "building proto"
	@for c in ${TOGEN}; do \
		printf "\n +++++++++++++++++ Generating $${c} +++++++++++++++++ \n"; \
		protoc \
		--proto_path=. \
		-I/usr/src/github.com/ROCm/device-metrics-exporter/vendor \
		--go-patch_out=plugin=go,paths=import:$(PWD)/pkg/amdnic \
		--go-patch_out=plugin=go-grpc,paths=import:$(PWD)/pkg/amdnic \
		$${c} || exit 1; \
	done;

.PHONY: all
all: bld_proto