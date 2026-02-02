package help

import (
	"flag"
	"os"
	"testing"

	"github.com/justchokingaround/greg/internal/tui/tuitest"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestHelpView(t *testing.T) {
	model := New()
	model.SetProviderName("test-provider")
	model.width = 80
	model.height = 30
	model.Show()

	view := model.View()

	tuitest.AssertSnapshot(t, view)
}
