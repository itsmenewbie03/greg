# CLAUDE.md

Guidance for AI assistants working with the greg codebase.

## Project Overview

**greg** is a unified terminal UI for streaming anime, movies, TV shows and reading manga with AniList progress tracking.

- Built with Go 1.25.3
- TUI framework: Bubble Tea (charmbracelet/bubbletea)
- Player: mpv via gopv IPC library
- Database: SQLite with GORM
- Config: Viper with XDG support

## Quick Reference

```bash
# Essential commands
just build          # Build dev binary
just test           # Run unit tests
just lint           # Run linters
just fmt            # Format code
just pre-commit     # fmt + lint + test (run before commits)

# Development
just dev            # Hot reload with air
just run            # Build and run
just doctor         # Check dev environment

# Testing
just test-coverage      # Generate coverage report
just test-integration   # Integration tests (requires network)
just test-race          # Race condition detection
just test-pkg <name>    # Test specific package
```

## Architecture

```
cmd/greg/           Entry point (main.go)
internal/
├── tui/            Bubble Tea UI
│   ├── model.go    Main app model (1,377 lines, refactored from 4,822)
│   ├── *_handlers.go  Message handlers (organized by domain)
│   │   ├── keyboard_handlers.go     Keyboard input routing
│   │   ├── navigation_handlers.go   Navigation messages
│   │   ├── media_message_handlers.go  Media operations
│   │   ├── anilist_handlers.go      AniList integration
│   │   ├── download_handlers.go     Download operations
│   │   ├── watchparty_handlers.go   WatchParty features
│   │   ├── manga_handlers.go        Manga operations
│   │   ├── debug_handlers.go        Debug utilities
│   │   └── state_handlers.go        State management
│   ├── playback.go  Playback operations (1,375 lines)
│   ├── components/ Individual views (home, search, results, etc.)
│   └── styles/     Oxocarbon color scheme
├── providers/      Streaming providers
│   ├── provider.go Core interface
│   ├── registry.go Provider registration
│   ├── hianime/    Default anime provider
│   ├── allanime/   Alternative anime
│   ├── sflix/      Default movies/TV
│   ├── flixhq/     Alternative movies/TV
│   ├── hdrezka/    Russian provider (multi-language)
│   └── mangaprovider/comix/  Manga provider
├── player/mpv/     mpv integration via gopv
├── tracker/anilist/ AniList OAuth2 + GraphQL
├── downloader/     Download manager with worker pool
├── database/       SQLite models (history, downloads, mappings)
└── config/         Viper config + slog logger
pkg/
├── extractors/     Stream URL extractors (megacloud, vidcloud)
└── interfaces/     Shared interfaces
```

## Provider Interface

All providers implement:

```go
type Provider interface {
    Name() string
    Type() MediaType  // Anime, Movie, TV, MovieTV, Manga, All
    Search(ctx context.Context, query string) ([]Media, error)
    GetTrending(ctx context.Context) ([]Media, error)
    GetRecent(ctx context.Context) ([]Media, error)
    GetMediaDetails(ctx context.Context, id string) (*MediaDetails, error)
    GetSeasons(ctx context.Context, mediaID string) ([]Season, error)
    GetEpisodes(ctx context.Context, seasonID string) ([]Episode, error)
    GetStreamURL(ctx context.Context, episodeID string, quality Quality) (*StreamURL, error)
    GetAvailableQualities(ctx context.Context, episodeID string) ([]Quality, error)
    IsAvailable(ctx context.Context) bool
}
```

**Current providers:**
| Provider | Type | Default | Notes |
|----------|------|---------|-------|
| hianime | Anime | Yes | Fast search (~62ms) |
| allanime | Anime | No | Multiple sources per episode |
| hdrezka | Anime/Movies/TV | No | Multi-language, mostly Russian |
| sflix | Movies/TV | Yes | Large library |
| flixhq | Movies/TV | No | Multiple servers |
| comix | Manga | Yes | Default manga provider |

## Key Patterns

### Provider Development

1. Create package in `internal/providers/<name>/`
2. Implement Provider interface
3. Use `sync.Map` for response caching
4. Handle movies vs TV (empty seasons array for movies)
5. Register in `internal/providers/registry.go`
6. Write tests with `httptest.NewServer` for mocking

**Important:**
- Always use `context.Context` as first parameter
- Wrap errors: `fmt.Errorf("operation: %w", err)`
- Never cache stream URLs (they expire)
- Cache media info to reduce requests

### mpv Integration

Uses gopv

```go
// Platform-specific IPC
// Linux/macOS/WSL: Unix sockets /tmp/greg-mpv-{random}.sock
// Windows: Named pipes \\.\pipe\greg-mpv-{random}

// Key flow:
// 1. Generate socket path
// 2. Launch mpv with --input-ipc-server=<socket>
// 3. Connect via gopv.Connect() with timeout
// 4. Background goroutine polls progress every 2 seconds
// 5. Auto-return to UI when playback ends
```

### TUI Architecture

**Package Organization (Refactored Jan 2026):**

The `internal/tui` package was refactored from a monolithic 4,822-line `model.go` into a modular structure:

- **model.go** (1,377 lines) - Core App struct, Update/View methods, state management
- **Handler files** (by domain):
  - `keyboard_handlers.go` - Keyboard input routing with component delegation
  - `navigation_handlers.go` - GoToSearch, GoToHome, Back, etc.
  - `media_message_handlers.go` - Search, seasons, episodes, media selection
  - `anilist_handlers.go` - AniList library, tracking, mapping
  - `download_handlers.go` - Download operations and notifications
  - `watchparty_handlers.go` - WatchParty URL generation and sharing
  - `manga_handlers.go` - Manga info and chapter navigation
  - `debug_handlers.go` - Debug popup and source inspection
  - `state_handlers.go` - Simple state transitions
- **playback.go** (1,375 lines) - Playback operations, progress tracking, resume logic

All methods remain on the `App` struct - no architectural changes, just better organization.

Bubble Tea message-passing model:

```go
// View states (internal/tui/model.go)
type sessionState int
const (
    homeView sessionState = iota
    searchView
    resultsView
    loadingView
    errorView
    seasonView
    episodeView
    launchingPlayerView
    playingView
    playbackCompletedView
    anilistView
    providerSelectionView
    downloadsView
    historyView
    mangaReaderView
    mangaInfoView
    providerStatusView
    mangaDownloadProgressView
    // 18 states total
)

// Smart navigation:
// - Auto-skip season selection for single-season content
// - Auto-skip episode selection for single-episode content (movies)
// - Direct movie playback without selection screens
```

### Database Models

SQLite tables (internal/database/models.go):

- `history` - Watch history with progress tracking
- `statistics` - Total watch time, genre stats
- `sync_queue` - Pending AniList syncs
- `downloads` - Download queue and status
- `settings` - Key-value config cache
- `anilist_mappings` - Provider to AniList media mappings

## Configuration

**Hierarchy** (from lowest to highest importance):
1. Hard-coded defaults
2. `~/.config/greg/config.yaml`
3. Environment variables (`GREG_*`)
4. Command-line flags

**XDG directories:**
- Config: `$XDG_CONFIG_HOME/greg/` or `~/.config/greg/`
- Data: `$XDG_DATA_HOME/greg/` or `~/.local/share/greg/`
- Cache: `$XDG_CACHE_HOME/greg/` or `~/.cache/greg/`

**Defaults work out of the box.** No configuration required for basic usage.

### API Server (Optional)

The external API server at `localhost:8080` is **optional and experimental**. It is NOT required.

- Default mode: `local` - providers use internal scraping
- Optional mode: `remote` - delegate to external API server

This feature is not fully supported yet. When working with providers, assume local mode unless explicitly told otherwise.

## Known Issues & TODOs

### TODOs

- [ ] Verify provider health checks actually work
- [ ] Change manga DB storage to use chapters instead of reusing episodes field
- [ ] Add CHANGELOG

### Not Production Ready

**Manga reading mode** has various bugs and UX issues. It works but needs polish before considered stable.

## Code Style

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `just fmt` before commits (gofmt + goimports)
- Run `just lint` (golangci-lint)
- Context as first parameter
- Wrap errors: `fmt.Errorf("context: %w", err)`
- Never panic in library code
- Comment all exported types/functions
- Aim for 80%+ test coverage on new code

## Testing

```bash
just test                # Unit tests
just test-integration    # Requires network
just test-race           # Race detector
just test-coverage       # HTML report
just test-pkg providers  # Specific package
```

**Writing tests:**
- Unit tests: Mock HTTP with `httptest.NewServer`
- Table-driven tests for parsing/validation
- Integration tests: Tag with `//go:build integration`
- TUI snapshot tests exist in `internal/tui/`

## Dependencies

**Core libraries:**
- cobra - CLI framework
- viper - Configuration
- bubbletea, lipgloss, bubbles - TUI
- goquery - HTML parsing
- gorm - SQLite ORM
- oauth2 - AniList auth
- gopv - mpv IPC (local fork)

**External tools:**
- mpv (required) - Video playback
- ffmpeg (required for downloads) - Subtitle embedding
- air (optional) - Hot reload

## Important Notes

1. **Version info** injected via ldflags at build time (see justfile)
2. **Entry point** is `cmd/greg/main.go`
3. **AniList sync** triggers at 85% watch threshold
4. **Platform detection** is automatic (Linux/macOS/Windows/WSL)
5. **WSL** uses Linux mpv (not Windows mpv.exe) for better IPC compatibility

## Documentation

- `README.org` - User documentation
- `docs/dev/ARCHITECTURE.org` - Detailed system design
- `docs/PROVIDERS.org` - Provider development guide
- `docs/CONFIG.org` - Configuration reference
- `docs/dev/CONTRIBUTING.org` - Contribution guidelines
- `AGENTS.md` - AI agent descriptions for this project
