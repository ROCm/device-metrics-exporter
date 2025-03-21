.PHONY: mock_gen
TO_MOCK := gpu events
mock_gen:
	@echo "building mock gen"
	@mkdir -p ${MOCK_DIR}
	@for c in ${TO_MOCK}; do printf "\n+++++++++++++++++ Generating mock $${c} +++++++++++++++++\n"; mockgen --destination=${MOCK_DIR}/mock_$${c}.go -package=mock_gen --source=$(GEN_DIR)/gen/amdgpu/$${c}_grpc.pb.go; done;
	@goimports -local "github.com/ROCm/device-metrics-exporter" -l ${MOCK_DIR}

.PHONY: all
all: mock_gen
