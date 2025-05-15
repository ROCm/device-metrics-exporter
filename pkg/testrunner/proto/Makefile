.PHONY: bld_proto
TOGEN := $(shell ls *.proto)
bld_proto:
	@echo "building proto"
	@mkdir -p ${GEN_DIR}
	@for c in ${TOGEN}; do printf "\n +++++++++++++++++ Generating $${c} \n"; protoc --proto_path=. --go-grpc_out=$(GEN_DIR) --go_out=$(GEN_DIR) $${c}; done;

.PHONY: all
all: bld_proto