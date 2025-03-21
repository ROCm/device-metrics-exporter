.PHONY: mock_gen
mock_gen:
	@echo "building mock gen"
	@mkdir -p ${MOCK_DIR}
	mockgen --destination=${MOCK_DIR}/mock_gpu.go -package=mock_gen --source=$(GEN_DIR)/gen/amdgpu/gpu_grpc.pb.go
	goimports -local "github.com/ROCm/device-metrics-exporter" -w ${MOCK_DIR}/mock_gpu.go

.PHONY: all
all: mock_gen
