package clipboard

import (
	"context"
	"testing"

	"github.com/justchokingaround/greg/internal/config"
)

// MockLogger implements the Logger interface for testing
type MockLogger struct{}

func (m *MockLogger) Debug(msg string, keyvals ...interface{}) {}

func (m *MockLogger) Warn(msg string, keyvals ...interface{}) {}

func (m *MockLogger) Error(msg string, keyvals ...interface{}) {}

func TestNewService(t *testing.T) {
	logger := &MockLogger{}
	service := NewService(logger)

	if service == nil {
		t.Fatal("Expected service to be non-nil")
	}

	// Test that it implements the Service interface
	var _ Service = service
}

func TestService_Read_Write(t *testing.T) {
	logger := &MockLogger{}
	service := NewService(logger)

	// Test with nil config
	_, err := service.Read(context.Background(), nil)
	if err == nil {
		t.Error("Expected error when config is nil")
	}

	// Test write with nil config
	cmd := service.Write(context.Background(), "test", nil)
	if cmd == nil {
		t.Error("Expected cmd to be non-nil")
	}
}

func TestService_Write_WithConfig(t *testing.T) {
	logger := &MockLogger{}
	service := NewService(logger)

	cfg := &config.Config{
		Advanced: config.AdvancedConfig{
			Clipboard: config.ClipboardConfig{
				Command: "echo", // Use echo as a safe test command
			},
		},
	}

	// This should not panic and should return a command
	cmd := service.Write(context.Background(), "test", cfg)
	if cmd == nil {
		t.Error("Expected cmd to be non-nil")
	}
}
