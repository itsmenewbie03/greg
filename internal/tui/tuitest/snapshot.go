package tuitest

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update snapshot files")

func AssertSnapshot(t *testing.T, output string) {
	t.Helper()

	snapshotPath := filepath.Join("testdata", strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))+".snap")

	if *update {
		err := os.MkdirAll(filepath.Dir(snapshotPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(snapshotPath, []byte(output), 0644)
		require.NoError(t, err)
		t.Logf("updated snapshot: %s", snapshotPath)
		return
	}

	snapshot, err := os.ReadFile(snapshotPath)
	if os.IsNotExist(err) {
		t.Fatalf("snapshot file not found: %s. run with -update to create it.", snapshotPath)
	}
	require.NoError(t, err)

	require.Equal(t, string(snapshot), output, "snapshot does not match. run with -update to update it.")
}
