package api

// SearchResponse represents the API's search response
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// SearchResult represents a single search result from the API
type SearchResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Image       string `json:"image"`
	URL         string `json:"url"`         // Can contain type info (e.g., /movie/ or /tv/)
	ReleaseDate string `json:"releaseDate"` // Release date from API (YYYY-MM-DD format)
	Type        string `json:"type"`        // "Movie" or "TV Series"
}

// InfoResponse represents the API's info/details response
type InfoResponse struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	URL           string         `json:"url"`
	Image         string         `json:"image"`
	Description   string         `json:"description"`
	Genres        []string       `json:"genres"`
	Status        string         `json:"status"`
	TotalEpisodes int            `json:"totalEpisodes"`
	Episodes      []APIEpisode   `json:"episodes"`
	Chapters      []MangaChapter `json:"chapters,omitempty"` // For manga providers
	ReleaseDate   string         `json:"releaseDate,omitempty"`
	Rating        string         `json:"rating,omitempty"`     // IMDB rating (e.g., "7.5")
	Type          string         `json:"type,omitempty"`       // For movies: "Movie" or "TV Series"
	LastSeason    int            `json:"lastSeason,omitempty"` // For TV shows: last season number
}

// APIEpisode represents an episode in the API response
type APIEpisode struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url,omitempty"`    // Contains mediaID for some providers (e.g., SFlix)
	Season int    `json:"season,omitempty"` // Some providers include season info
}

// Server represents a streaming server option
type Server struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SourcesResponse represents the API's sources response
type SourcesResponse struct {
	Sources   []Source       `json:"sources"`
	Subtitles []Subtitle     `json:"subtitles"`
	Headers   *SourceHeaders `json:"headers,omitempty"`
}

// Source represents a video source
type Source struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
	IsM3U8  bool   `json:"isM3U8"`
	Referer string `json:"referer,omitempty"` // Optional referer from source (HiAnime)
	Origin  string `json:"origin,omitempty"`  // Optional origin from source
}

// Subtitle represents a subtitle track
type Subtitle struct {
	URL  string `json:"url"`
	Lang string `json:"lang"`
}

// SourceHeaders contains headers required for video playback
type SourceHeaders struct {
	Referer string `json:"Referer,omitempty"`
	Origin  string `json:"Origin,omitempty"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// MangaChapter represents a manga chapter in the API response
type MangaChapter struct {
	ID     string `json:"id"`
	Number string `json:"number"` // Can be "1", "1.5", etc.
	Title  string `json:"title"`
}

// MangaPagesResponse represents the API's manga pages response
type MangaPagesResponse struct {
	Pages []MangaPage `json:"pages"`
}

// MangaPage represents a single page of a manga chapter
type MangaPage struct {
	URL   string `json:"url"`
	Index int    `json:"index"`
}
