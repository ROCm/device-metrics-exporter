package gpuagent

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/prometheus/client_golang/prometheus"
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
	exists := fl.checkUnsupportedFields("test_field")
	if exists {
		t.Error("checkUnsupportedFields should return false for non-existent field")
	}

	// Add a field manually and test
	fl.unsupportedFieldMap["existing_field"] = true
	exists = fl.checkUnsupportedFields("existing_field")
	if !exists {
		t.Error("checkUnsupportedFields should return true for existing field")
	}

	// Test with nil map
	fl.unsupportedFieldMap = nil
	exists = fl.checkUnsupportedFields("any_field")
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
	fl.logUnsupportedField("test_field")

	if !fl.unsupportedFieldMap["test_field"] {
		t.Error("Field should be marked as unsupported")
	}

	output := buf.String()
	if !strings.Contains(output, "Platform doesn't support field name: test_field") {
		t.Errorf("Expected log message not found, got %v", output)
	}

	// Test logging the same field again (should not log twice)
	buf.Reset()
	fl.logUnsupportedField("test_field")

	output = buf.String()
	if output != "" {
		t.Error("Should not log the same field twice")
	}

	// Test with nil map
	fl.unsupportedFieldMap = nil
	buf.Reset()
	fl.logUnsupportedField("new_field")

	if fl.unsupportedFieldMap == nil {
		t.Error("unsupportedFieldMap should be initialized")
	}

	if !fl.unsupportedFieldMap["new_field"] {
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
	fl.logWithValidateAndExport(*testMetric, "unsupported_field", labels, 123.45)

	output := buf.String()
	if output != "" {
		t.Error("Should not log anything for already unsupported fields")
	}

	// Test with valid field and value
	buf.Reset()
	fl.logWithValidateAndExport(*testMetric, "supported_field", labels, 123.45)

	// Since we can't easily mock utils.ValidateAndExport, we'll test the logging behavior
	// when it would return errors by testing the method exists and can be called

	// Verify the field wasn't marked as unsupported for valid case
	if fl.checkUnsupportedFields("supported_field") {
		t.Error("Valid field should not be marked as unsupported")
	}
}

func TestFieldLoggerConcurrency(t *testing.T) {
	fl := NewFieldLogger()

	// Test concurrent access to avoid race conditions
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			fl.logUnsupportedField("field1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fl.checkUnsupportedFields("field1")
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	if !fl.checkUnsupportedFields("field1") {
		t.Error("Field1 should be marked as unsupported after concurrent operations")
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
