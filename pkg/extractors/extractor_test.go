package extractors

import (
	"testing"
)

func TestGetExtractor(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		wantType   string
	}{
		{
			name:       "vidcloud server",
			serverName: "Vidcloud",
			wantType:   "*extractors.VidCloudExtractor",
		},
		{
			name:       "upcloud server",
			serverName: "UpCloud",
			wantType:   "*extractors.VidCloudExtractor",
		},
		{
			name:       "megacloud server",
			serverName: "MegaCloud",
			wantType:   "*extractors.VidCloudExtractor",
		},
		{
			name:       "akcloud server",
			serverName: "AKCloud",
			wantType:   "*extractors.VidCloudExtractor",
		},
		{
			name:       "unknown server defaults to vidcloud",
			serverName: "UnknownServer",
			wantType:   "*extractors.VidCloudExtractor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := GetExtractor(tt.serverName)
			if extractor == nil {
				t.Error("GetExtractor() returned nil")
				return
			}

			// Type check
			switch extractor.(type) {
			case *VidCloudExtractor:
				// Expected
			default:
				t.Errorf("GetExtractor() returned unexpected type %T", extractor)
			}
		})
	}
}

func TestNewVidCloudExtractor(t *testing.T) {
	extractor := NewVidCloudExtractor()

	if extractor == nil {
		t.Error("NewVidCloudExtractor() returned nil")
		return
	}

	if extractor.Client == nil {
		t.Error("HTTP Client is nil")
	}
}
