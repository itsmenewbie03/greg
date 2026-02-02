package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/anilist"
	"github.com/justchokingaround/greg/internal/tui/components/downloads"
)

func (a *App) handleGoToSearchMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = searchView
	return a, a.search.Init()
}

func (a *App) handleGoToHomeMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = homeView
	a.cameFromHistory = false
	return a, a.home.Init()
}

func (a *App) handleBackMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	var needsHistoryRefresh bool
	switch a.state {
	case searchView:
		a.state = homeView
		needsHistoryRefresh = true
	case resultsView:
		a.state = searchView
	case seasonView:
		a.state = resultsView
		if a.watchingFromAniList {
			a.state = anilistView
		}
	case episodeView:
		if a.cameFromHistory {
			a.state = historyView
			a.cameFromHistory = false
		} else if len(a.seasonsList) > 1 {
			a.state = seasonView
		} else {
			if a.watchingFromAniList {
				a.state = anilistView
			} else {
				a.state = resultsView
			}
		}
	case providerSelectionView:
		if a.watchingFromAniList {
			a.state = anilistView
		} else {
			if a.previousState != -1 {
				a.state = a.previousState
				a.previousState = -1
			} else {
				a.state = searchView
			}
		}
	case anilistView:
		a.state = homeView
		needsHistoryRefresh = true
	case mangaInfoView:
		if a.previousState != -1 {
			a.state = a.previousState
			a.previousState = -1
		} else {
			a.state = homeView
			needsHistoryRefresh = true
		}
	}
	if needsHistoryRefresh {
		return a, func() tea.Msg {
			return common.RefreshHistoryMsg{}
		}
	}
	return a, nil
}

func (a *App) handleToggleProviderMsg() (tea.Model, tea.Cmd) {
	if a.searchQueries == nil {
		a.searchQueries = make(map[providers.MediaType]string)
	}
	a.searchQueries[a.currentMediaType] = a.search.GetValue()

	switch a.currentMediaType {
	case providers.MediaTypeMovieTV:
		a.currentMediaType = providers.MediaTypeAnime
	case providers.MediaTypeAnime:
		a.currentMediaType = providers.MediaTypeManga
	default:
		a.currentMediaType = providers.MediaTypeMovieTV
	}
	a.home.CurrentMediaType = a.currentMediaType
	if provider, ok := a.providers[a.currentMediaType]; ok {
		a.home.SetProvider(provider.Name())
		a.helpComponent.SetProviderName(provider.Name())
	}

	if query, ok := a.searchQueries[a.currentMediaType]; ok {
		a.search.SetValue(query)
	} else {
		a.search.SetValue("")
	}

	return a, a.home.Init()
}

func (a *App) handleGoToAniListMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = loadingView
	a.loadingOp = loadingAniListLibrary
	return a, tea.Batch(a.spinner.Tick, a.fetchAniListLibrary())
}

func (a *App) handleGoToDownloadsMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = downloadsView
	a.quitRequested = false
	return a, a.downloadsComponent.Refresh()
}

func (a *App) handleGoToProviderStatusMsg() (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = providerStatusView
	return a, a.providerStatusComponent.Init()
}

func (a *App) handleDownloadsTickMsg(msg common.DownloadsTickMsg) (tea.Model, tea.Cmd) {
	if a.state == downloadsView {
		var newModel tea.Model
		var cmd tea.Cmd
		newModel, cmd = a.downloadsComponent.Update(msg)
		a.downloadsComponent = newModel.(downloads.Model)
		return a, cmd
	}
	return a, nil
}

func (a *App) handleGoToHistoryMsg(msg common.GoToHistoryMsg) (tea.Model, tea.Cmd) {
	a.statusMsg = ""
	a.state = historyView
	a.cameFromHistory = false
	a.historyComponent.Reset()
	a.historyComponent.SetMediaTypeFilter(msg.MediaType)
	return a, a.historyComponent.Init()
}

func (a *App) handleAnilistBackMsg() (tea.Model, tea.Cmd) {
	a.state = homeView
	return a, func() tea.Msg {
		return common.RefreshHistoryMsg{}
	}
}

func (a *App) handleLibraryLoadedMsg(msg anilist.LibraryLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		a.err = msg.Error
		a.state = errorView
		return a, nil
	}

	a.anilistComponent.SetLibrary(msg.Library)
	a.state = anilistView
	return a, tea.ClearScreen
}
