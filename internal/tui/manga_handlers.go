package tui

// This file contains manga-related message handlers extracted from model.go
// for better code organization. All methods remain on the App struct.

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/providers"
	"github.com/justchokingaround/greg/internal/scraper"
	"github.com/justchokingaround/greg/internal/tui/common"
	"github.com/justchokingaround/greg/internal/tui/components/mangainfo"
)

func (a *App) handleMangaInfoMsg(msg common.MangaInfoMsg) (tea.Model, tea.Cmd) {
	a.previousState = a.state
	a.state = loadingView
	a.loadingOp = loadingMangaInfo
	return a, tea.Batch(
		a.spinner.Tick,
		func() tea.Msg {
			info, err := scraper.GetMangaInfo(msg.AnimeTitle)
			return common.MangaInfoResultMsg{
				Info: info,
				Err:  err,
			}
		},
	)
}

func (a *App) handleMangaInfoResultMsg(msg common.MangaInfoResultMsg) (tea.Model, tea.Cmd) {
	a.state = mangaInfoView
	// Ensure component has correct size
	a.mangaInfoComponent.SetSize(a.width, a.height)
	// Pass the message to the component
	var cmd tea.Cmd
	var model tea.Model
	model, cmd = a.mangaInfoComponent.Update(msg)
	a.mangaInfoComponent = model.(*mangainfo.Model)
	return a, cmd
}

func (a *App) handleNextChapterMsg(msg common.NextChapterMsg) (tea.Model, tea.Cmd) {
	// Find next chapter
	targetChapter := a.currentEpisodeNumber + 1
	var nextChapter *providers.Episode
	for i := range a.episodes {
		if a.episodes[i].Number == targetChapter {
			nextChapter = &a.episodes[i]
			break
		}
	}

	if nextChapter != nil {
		// Load next chapter
		return a, func() tea.Msg {
			return common.EpisodeSelectedMsg{
				EpisodeID: nextChapter.ID,
				Number:    nextChapter.Number,
				Title:     nextChapter.Title,
			}
		}
	}

	// No more chapters
	a.statusMsg = "No more chapters available."
	a.statusMsgTime = time.Now()
	return a, func() tea.Msg {
		time.Sleep(3 * time.Second)
		return clearStatusMsg{}
	}
}
