package gpuagent

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewFieldLogger(t *testing.T) {
	fl := NewFieldLogger()

	if fl == nil {
		t.Fatal("NewFieldLogger returned nil")
	}

	if fl.unsupportedFieldMap == nil {
		t.Error("unsupportedFieldMap should be initialized")
	}

	if len(fl.unsupportedFieldMap) != 0 {
		t.Error("unsupportedFieldMap should be empty initially")
	}
}

func TestCheckUnsupportedFields(t *testing.T) {
	fl := NewFieldLogger()

	// Test with empty map
	exists := fl.checkUnsupportedFields("0", "test_field")
	if exists {
		t.Error("checkUnsupportedFields should return false for non-existent field")
	}

	// Add a field manually and test
	fl.unsupportedFieldMap["0-existing_field"] = true
	exists = fl.checkUnsupportedFields("0", "existing_field")
	if !exists {
		t.Error("checkUnsupportedFields should return true for existing field")
	}

	// Test with nil map
	fl.unsupportedFieldMap = nil
	exists = fl.checkUnsupportedFields("0", "any_field")
	if exists {
		t.Error("checkUnsupportedFields should return false when map is nil")
	}
}

func TestLogUnsupportedField(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	logger.Log.SetOutput(&buf)

	fl := NewFieldLogger()

	// Test logging a new unsupported field
	fl.logUnsupportedField("0", "test_field")

	if !fl.unsupportedFieldMap["0-test_field"] {
		t.Error("Field should be marked as unsupported")
	}

	output := buf.String()
	if !strings.Contains(output, "Platform doesn't support field name: test_field") {
		t.Errorf("Expected log message not found, got %v", output)
	}

	// Test logging the same field again (should not log twice)
	buf.Reset()
	fl.logUnsupportedField("0", "test_field")

	output = buf.String()
	if output != "" {
		t.Error("Should not log the same field twice")
	}

	// Test with nil map
	fl.unsupportedFieldMap = nil
	buf.Reset()
	fl.logUnsupportedField("0", "new_field")

	if fl.unsupportedFieldMap == nil {
		t.Error("unsupportedFieldMap should be initialized")
	}

	if !fl.unsupportedFieldMap["0-new_field"] {
		t.Error("Field should be marked as unsupported after map initialization")
	}
}

func TestLogWithValidateAndExport(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	logger.Log.Log = log.New(&buf, "", 0)

	fl := NewFieldLogger()

	// Create a test metric
	testMetric := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_metric",
			Help: "Test metric for unit testing",
		},
		[]string{"label1", "label2"},
	)

	labels := map[string]string{
		"label1": "value1",
		"label2": "value2",
	}

	// Test with unsupported field (should not call ValidateAndExport)
	fl.unsupportedFieldMap["unsupported_field"] = true
	buf.Reset()
	fl.logWithValidateAndExport("0", *testMetric, "unsupported_field", labels, 123.45)

	output := buf.String()
	if output != "" {
		t.Error("Should not log anything for already unsupported fields")
	}

	// Test with valid field and value
	buf.Reset()
	fl.logWithValidateAndExport("0", *testMetric, "supported_field", labels, 123.45)

	// Since we can't easily mock utils.ValidateAndExport, we'll test the logging behavior
	// when it would return errors by testing the method exists and can be called

	// Verify the field wasn't marked as unsupported for valid case
	if fl.checkUnsupportedFields("0", "supported_field") {
		t.Error("Valid field should not be marked as unsupported")
	}
}

func TestFieldLoggerConcurrency(t *testing.T) {
	fl := NewFieldLogger()

	// Test concurrent access to avoid race conditions
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			gpu_id := fmt.Sprintf("%d", i)
			fl.logUnsupportedField(gpu_id, "field1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			gpu_id := fmt.Sprintf("%d", i)
			fl.checkUnsupportedFields(gpu_id, "field1")
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	if !fl.checkUnsupportedFields("0", "field1") {
		t.Error("Field1 should be marked as unsupported after concurrent operations")
	}
}

// exportsAfterStartup runs the startup sample that decides field support and then a later valid
// reading, returning whether each one reached the Prometheus output.
func exportsAfterStartup(t *testing.T, field string, startup, later interface{}) (bool, bool) {
	t.Helper()
	fl := NewFieldLogger()
	metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "gpu_metric", Help: "h"}, []string{"gpu_id"})
	labels := map[string]string{"gpu_id": "0"}
	exported := func(value interface{}) bool {
		fl.logWithValidateAndExport("0", *metric, field, labels, value)
		return testutil.CollectAndCount(metric) == 1
	}
	first := exported(startup)
	fl.SetFilterDone()
	metric.Reset()
	return first, exported(later)
}

func TestSensorFieldsAtStartupRemainExported(t *testing.T) {
	// uint16 AMD-SMI sensors arrive widened (uint32, uint64 or float32); the shim maps the source
	// sentinel onto the wire type's own unavailable value and never narrows a reading.
	logger.Init(true)
	for _, tc := range []struct {
		name        string
		startup     interface{}
		wantStartup bool
	}{
		{"power 254 W", fromUint16Source(uint64(254)), true},
		{"power 255 W (UINT8_MAX in a wider type)", fromUint16Source(uint64(255)), true},
		{"power 256 W", fromUint16Source(uint64(256)), true},
		{"voltage 255 mV from uint32", fromUint16Source(uint32(255)), true},
		{"temperature 255 C from float32", fromUint16SourceFloat(255), true},
		{"uint16 sentinel", fromUint16Source(uint64(math.MaxUint16)), false},
		{"uint32 sentinel in a uint64", fromUint16Source(uint64(math.MaxUint32)), false},
		{"uint64 sentinel", fromUint16Source(uint64(math.MaxUint64)), false},
		{"uint32 sentinel in a uint32", fromUint16Source(uint32(math.MaxUint32)), false},
		{"float32 uint16 sentinel", fromUint16SourceFloat(math.MaxUint16), false},
		{"float32 uint32 sentinel", fromUint16SourceFloat(math.MaxUint32), false},
		{"float32 negative", fromUint16SourceFloat(-1), false},
		{"uint64 beyond uint16 range is unavailable, not wrapped", fromUint16Source(uint64(255_000_000)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, later := exportsAfterStartup(t, "GPU_POWER_USAGE", tc.startup, fromUint16Source(uint64(239)))
			if first != tc.wantStartup {
				t.Errorf("startup sample exported = %v; want %v", first, tc.wantStartup)
			}
			if later != tc.wantStartup {
				t.Errorf("later 239 exported = %v; want %v (field support is decided by the startup sample)", later, tc.wantStartup)
			}
		})
	}
}

func TestUint16SourceShimNeverAltersAReading(t *testing.T) {
	for _, v := range []uint64{0, 1, 254, 255, 256, 1000, math.MaxUint16 - 1} {
		if got := fromUint16Source(v); got != v {
			t.Errorf("fromUint16Source(%d) = %d", v, got)
		}
	}
	if got := fromUint16SourceFloat(43.5); got != 43.5 {
		t.Errorf("fromUint16SourceFloat(43.5) = %v", got)
	}
	if got := fromUint16Source(uint32(math.MaxUint16)); got != math.MaxUint32 {
		t.Errorf("uint32 sentinel mapped to %d, want MaxUint32", got)
	}
}

func TestNativeCountersAtNarrowMaximaRemainExported(t *testing.T) {
	// ECC, PCIe, XGMI and violation counters are native uint64: only UINT64_MAX is unavailable.
	// Before, a counter sitting at 255, 65535, INT32_MAX or UINT32_MAX vanished, and if that was the
	// startup sample the field stayed filtered for the life of the config.
	logger.Init(true)
	for _, tc := range []struct {
		value uint64
		want  bool
	}{
		{255, true},
		{math.MaxUint16, true},
		{math.MaxInt32, true},
		{math.MaxUint32, true},
		{math.MaxUint64, false},
	} {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			first, later := exportsAfterStartup(t, "GPU_ECC_CORRECT_TOTAL", tc.value, uint64(256))
			if first != tc.want || later != tc.want {
				t.Errorf("counter %d: startup exported=%v later exported=%v; want %v", tc.value, first, later, tc.want)
			}
		})
	}
	// A fallback that means "unavailable" is a real sentinel of the presented type.
	if first, _ := exportsAfterStartup(t, "GPU_CLOCK", uint32(math.MaxUint32), uint32(1400)); first {
		t.Errorf("uint32(MaxUint32) fallback was exported as a value")
	}
	// The GIM smi-lib INT32_MAX sentinel is still unavailable for a uint32 field.
	if first, _ := exportsAfterStartup(t, "GPU_MMA_ACTIVITY", uint32(math.MaxInt32), uint32(5)); first {
		t.Errorf("uint32(MaxInt32) was exported as a value")
	}
}

// Cleanup function to restore original logger
func TestMain(m *testing.M) {
	// Store original logger
	originalLogger := logger.Log

	// Run tests
	code := m.Run()

	// Restore original logger
	logger.Log = originalLogger

	os.Exit(code)
}
