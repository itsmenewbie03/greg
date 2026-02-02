package extractors

import (
	"testing"
)

func TestVidCloudExtractor(t *testing.T) {
	extractor := NewVidCloudExtractor()
	if extractor == nil {
		t.Fatal("NewVidCloudExtractor returned nil")
	}
	if extractor.Client == nil {
		t.Fatal("VidCloudExtractor client is nil")
	}
}
