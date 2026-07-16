package daemon

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathsUseDedicatedRuntimeSubdirectory(t *testing.T) {
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(paths.RuntimeDir) != "runtime" || filepath.Base(filepath.Dir(paths.RuntimeDir)) != stateDirectoryName {
		t.Fatalf("runtime directory = %q, want .../%s/runtime", paths.RuntimeDir, stateDirectoryName)
	}
	if filepath.Dir(paths.MetadataFile) != paths.RuntimeDir || filepath.Dir(paths.LockFile) != paths.RuntimeDir || filepath.Dir(paths.LogFile) != paths.RuntimeDir {
		t.Fatalf("daemon files do not share runtime directory: %#v", paths)
	}
}
