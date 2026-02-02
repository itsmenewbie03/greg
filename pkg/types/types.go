package types

// Common types
type MediaStatus string

const (
	MediaStatusOngoing   MediaStatus = "ONGOING"
	MediaStatusCompleted MediaStatus = "COMPLETED"
	MediaStatusUnknown   MediaStatus = "UNKNOWN"
)

type MediaFormat string

const (
	MediaFormatTV      MediaFormat = "TV"
	MediaFormatMovie   MediaFormat = "MOVIE"
	MediaFormatOVA     MediaFormat = "OVA"
	MediaFormatONA     MediaFormat = "ONA"
	MediaFormatSpecial MediaFormat = "SPECIAL"
)

// Search result types
type SearchResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Image       string `json:"image,omitempty"`
	URL         string `json:"url,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Type        string `json:"type,omitempty"`
}

type SearchResults struct {
	Results []SearchResult `json:"results"`
}

// Episode types
type Episode struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Season int    `json:"season,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Server types
type EpisodeServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Source types
type Source struct {
	URL     string `json:"url"`
	Quality string `json:"quality,omitempty"`
	IsM3U8  bool   `json:"isM3U8,omitempty"`
	Referer string `json:"referer"`
}

type Subtitle struct {
	URL  string `json:"url"`
	Lang string `json:"lang"`
}

type VideoSources struct {
	Sources   []Source   `json:"sources"`
	Subtitles []Subtitle `json:"subtitles"`
}

// Info types
type AnimeInfo struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	URL           string      `json:"url,omitempty"`
	Image         string      `json:"image,omitempty"`
	Description   string      `json:"description,omitempty"`
	Genres        []string    `json:"genres,omitempty"`
	Status        MediaStatus `json:"status,omitempty"`
	TotalEpisodes int         `json:"totalEpisodes,omitempty"`
	ReleaseDate   string      `json:"releaseDate,omitempty"`
	Type          MediaFormat `json:"type,omitempty"`
	Episodes      []Episode   `json:"episodes,omitempty"`
}

type MovieInfo struct {
	ID                      string    `json:"id"`
	Title                   string    `json:"title"`
	URL                     string    `json:"url,omitempty"`
	Image                   string    `json:"image,omitempty"`
	Description             string    `json:"description,omitempty"`
	Genres                  []string  `json:"genres,omitempty"`
	ReleaseDate             string    `json:"releaseDate,omitempty"`
	Rating                  string    `json:"rating,omitempty"`
	Type                    string    `json:"type,omitempty"`
	LastSeason              int       `json:"lastSeason,omitempty"`
	TotalEpisodesLastSeason int       `json:"totalEpisodesLastSeason,omitempty"`
	Episodes                []Episode `json:"episodes,omitempty"`
}

type MangaChapter struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number string `json:"number"`
}

type MangaInfo struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	URL         string         `json:"url,omitempty"`
	Image       string         `json:"image,omitempty"`
	Description string         `json:"description,omitempty"`
	Genres      []string       `json:"genres,omitempty"`
	Status      MediaStatus    `json:"status,omitempty"`
	Chapters    []MangaChapter `json:"chapters,omitempty"`
}

type MangaPage struct {
	URL   string `json:"url"`
	Index int    `json:"index"`
}

type MangaPages struct {
	Pages []*MangaPage `json:"pages"`
}
