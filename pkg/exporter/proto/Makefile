.PHONY: bld_proto
TOGEN := $(shell ls *.proto)
PR_GEN_DIR := $(PWD)/pkg/exporter/
bld_proto:
	@echo "building proto"
	@mkdir -p ${PR_GEN_DIR}
	@for c in ${TOGEN}; do printf "\n +++++++++++++++++ Generating $${c} \n"; protoc --proto_path=. --go-grpc_out=$(PR_GEN_DIR) --go_out=$(PR_GEN_DIR) $${c}; done;

.PHONY: all
all: bld_proto
