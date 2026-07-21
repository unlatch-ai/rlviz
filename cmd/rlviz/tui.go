package main

import (
	"context"
	"fmt"

	"github.com/TheSnakeFang/rlviz/internal/app"
	"github.com/TheSnakeFang/rlviz/internal/daemon"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	viewerTUI "github.com/TheSnakeFang/rlviz/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func runTUI(sourcePath, adapterPath string) error {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureRuntimeDir(); err != nil {
		return err
	}
	store, err := rolloutindex.Open(paths.IndexFile)
	if err != nil {
		return err
	}
	defer store.Close()
	indexed, err := app.IndexSource(context.Background(), store, sourcePath, adapterPath)
	if err != nil {
		return err
	}
	rows, err := viewerTUI.Load(context.Background(), store)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("indexed source %q contains no trajectories", indexed.Info.ID)
	}
	rows = rowsForSource(rows, indexed.Info.ID)
	if len(rows) == 0 {
		return fmt.Errorf("indexed source %q contains no trajectories", indexed.Info.ID)
	}
	_, err = tea.NewProgram(viewerTUI.New(rows), tea.WithAltScreen()).Run()
	return err
}

func rowsForSource(rows []viewerTUI.Row, sourceID string) []viewerTUI.Row {
	filtered := make([]viewerTUI.Row, 0, len(rows))
	for _, row := range rows {
		if row.SourceID == sourceID {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
