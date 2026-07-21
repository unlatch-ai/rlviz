package main

import (
	"testing"

	viewerTUI "github.com/TheSnakeFang/rlviz/internal/tui"
)

func TestRowsForSourceExcludesOtherIndexedSources(t *testing.T) {
	rows := []viewerTUI.Row{{SourceID: "old"}, {SourceID: "requested"}, {SourceID: "old"}}
	filtered := rowsForSource(rows, "requested")
	if len(filtered) != 1 || filtered[0].SourceID != "requested" {
		t.Fatalf("filtered rows = %#v", filtered)
	}
}
