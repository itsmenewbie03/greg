package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justchokingaround/greg/internal/clipboard"
	"github.com/justchokingaround/greg/internal/config"
)

// ReadFromClipboard reads content from the system clipboard
func ReadFromClipboard(svc clipboard.Service, cfg *config.Config) (string, error) {
	return svc.Read(context.Background(), cfg)
}

// copyToClipboard copies text to the system clipboard
func (a *App) copyToClipboard(text string) tea.Cmd {
	cfg, ok := a.cfg.(*config.Config)
	if !ok {
		// If config is not available, just return no-op
		return func() tea.Msg { return nil }
	}

	return a.clipboardSvc.Write(context.Background(), text, cfg)
}

// copyToClipboardWithNotification copies text to clipboard and shows a notification
func (a *App) copyToClipboardWithNotification(text, itemName string) tea.Cmd {
	cmd := a.copyToClipboard(text)
	a.statusMsg = "📋 " + itemName + " copied to clipboard"
	a.statusMsgTime = time.Now()

	return tea.Batch(cmd, func() tea.Msg {
		time.Sleep(2500 * time.Millisecond)
		return clearStatusMsg{}
	})
}
