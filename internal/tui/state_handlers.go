package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/anilist"
	"github.com/justchokingaround/greg/internal/tui/components/home"
)

func (a *App) handleRefreshHistoryMsg(msg common.RefreshHistoryMsg) (tea.Model, tea.Cmd) {
	a.state = homeView
	homeModel, cmd := a.home.Update(msg)
	a.home = homeModel.(*home.Model)
	return a, cmd
}

func (a *App) handleSearchNewAnimeMsg(msg anilist.SearchNewAnimeMsg) (tea.Model, tea.Cmd) {
	return a, nil
}

func (a *App) handleClearStatusMsg(msg clearStatusMsg) (tea.Model, tea.Cmd) {
	if time.Since(a.statusMsgTime) >= 1*time.Second {
		a.statusMsg = ""
		a.mangaComponent.StatusMessage = ""
	}
	return a, nil
}

func (a *App) handleDismissDownloadNotificationMsg(msg dismissDownloadNotificationMsg) (tea.Model, tea.Cmd) {
	a.showDownloadNotification = false
	a.downloadNotificationMsg = ""
	return a, nil
}
