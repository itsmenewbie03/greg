package extractors

import "github.com/justchokingaround/greg/pkg/types"

// Extractor is the interface that all extractors must implement
type Extractor interface {
	Extract(url string) (*types.VideoSources, error)
}
