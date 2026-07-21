package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

func TestIndexSourceCanonicalCachesWholeGroup(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join("..", "..", "fixtures", "canonical", "group.ndjson")
	first, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Refreshed || first.Info.ID == "" {
		t.Fatalf("first index = %#v", first)
	}
	trajectories, err := store.Trajectories(context.Background(), first.Info.ID)
	if err != nil || len(trajectories) != 2 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	second, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Refreshed || second.Info.ID != first.Info.ID {
		t.Fatalf("cached index = %#v", second)
	}
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: first.Info.ID, TrajectoryID: "traj-success"})
	if err != nil || len(page.Events) != 2 || page.Events[0].Value.Source == nil || page.Events[0].Value.Source.Path == "" {
		t.Fatalf("events=%#v err=%v", page, err)
	}
}

func TestIndexSourceAdapterRequiresTrustAndIndexes(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is unavailable")
	}
	t.Setenv("RLVIZ_CONFIG_DIR", t.TempDir())
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	adapter := filepath.Join("..", "..", "examples", "adapters", "simple-jsonl")
	source := filepath.Join("..", "..", "examples", "traces", "simple-agent.jsonl")
	if _, err := IndexSource(context.Background(), store, source, adapter); err == nil {
		t.Fatal("untrusted adapter was indexed")
	}
	plugin, err := plugins.Load(adapter)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	indexed, err := IndexSource(context.Background(), store, source, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !indexed.Refreshed || indexed.Info.Adapter != plugin.Path || indexed.Info.Fingerprint != plugin.Digest {
		t.Fatalf("indexed = %#v", indexed)
	}
	trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
	if err != nil || len(trajectories) != 1 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: indexed.Info.ID, TrajectoryID: trajectories[0].Value.ID})
	if err != nil || page.Total != 4 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestIndexSourceRefreshesChangedCanonicalFile(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := filepath.Join(t.TempDir(), "trace.ndjson")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := IndexSource(context.Background(), store, source, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(source, first.Info.ModTime.AddDate(0, 0, 1), first.Info.ModTime.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	second, err := IndexSource(context.Background(), store, source, "")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Refreshed {
		t.Fatal("changed source was not refreshed")
	}
}
