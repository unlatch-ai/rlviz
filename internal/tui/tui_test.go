package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/app"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func TestBrowseAndReadAt80x24(t *testing.T) {
	failure := false
	row := Row{SourceID: "source", Case: "demo case", Summary: rolloutindex.TrajectorySummary{Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: "trajectory-demo", Termination: "failed"}}, Success: &failure, ErrorCount: 1}, Events: []*model.Event{{ID: "start", Sequence: 0, Kind: "generation"}, {ID: "error", Sequence: 10, Kind: "error", AlignmentKey: "error:policy"}, {ID: "reward", Sequence: 20, Kind: "reward"}}}
	viewer := New([]Row{row})
	viewer.color = false
	if got := viewer.View(); !strings.Contains(got, "▸✕◆") || !strings.Contains(got, "trajectory-demo") {
		t.Fatalf("Browse view missing caterpillar: %s", got)
	}
	updated, _ := viewer.Update(tea.KeyMsg{Type: tea.KeyEnter})
	read := updated.(Model)
	if read.mode != "read" || read.event != 1 {
		t.Fatalf("Enter selected mode=%s event=%d", read.mode, read.event)
	}
	view := read.View()
	if lines := strings.Count(view, "\n") + 1; lines > 24 {
		t.Fatalf("Read rendered %d lines", lines)
	}
	if !strings.Contains(view, ">   10 ✕ error") {
		t.Fatalf("Read view missing selected error: %s", view)
	}
	updated, _ = read.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).mode != "browse" {
		t.Fatal("Esc did not return to Browse")
	}
}

func TestCodingAgentShapeShowsEpisodesRetriesAndCompaction(t *testing.T) {
	events := []*model.Event{
		{ID: "setup", Kind: "message", AlignmentKey: "episode:setup"},
		{ID: "reproduce", Kind: "tool", AlignmentKey: "episode:reproduce"},
		{ID: "diagnose", Kind: "error", AlignmentKey: "episode:diagnose"},
		{ID: "retry", Kind: "error", AlignmentKey: "stage:diagnose:test-failure"},
		{ID: "compact", Kind: "state", AlignmentKey: "context:compaction", Context: &model.Context{Operation: "compaction", Provenance: "source_native"}},
		{ID: "patch", Kind: "tool", AlignmentKey: "episode:patch"},
		{ID: "verify", Kind: "grader", AlignmentKey: "episode:verify"},
	}
	viewer := New([]Row{{Case: "synthetic bugfix", Summary: rolloutindex.TrajectorySummary{Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: "coding-bugfix"}}}, Events: events}})
	viewer.color = false
	updated, _ := viewer.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := updated.(Model).View()
	for _, want := range []string{"episodes: setup → reproduce → diagnose → patch → verify", "2 errors/retries", "1 compaction"} {
		if !strings.Contains(view, want) {
			t.Fatalf("read view missing %q:\n%s", want, view)
		}
	}
}

func TestSemanticColorsUseANSI16(t *testing.T) {
	failure := false
	row := Row{
		Summary: rolloutindex.TrajectorySummary{
			Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: "semantic-colors"}},
			Success:    &failure,
			ErrorCount: 2,
		},
		Events: []*model.Event{
			{ID: "context", Kind: "state", Context: &model.Context{Operation: "compaction", Provenance: "source_native"}},
			{ID: "policy", Kind: "error", AlignmentKey: "error:policy"},
			{ID: "infra", Kind: "error", AlignmentKey: "error:infrastructure"},
			{ID: "pass", Kind: "grader", Output: map[string]any{"verdict": "pass"}},
		},
	}
	viewer := New([]Row{row})
	viewer.color = true
	view := viewer.View()
	for _, want := range []string{ansiBlue + "~" + ansiReset, ansiRed + "✕" + ansiReset, ansiInfra + "✕" + ansiReset, ansiGreen + "◆" + ansiReset} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing semantic ANSI mark %q:\n%s", want, view)
		}
	}
}

func TestNOColorDisablesANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	viewer := New([]Row{{
		Summary: rolloutindex.TrajectorySummary{Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: "plain"}}},
		Events:  []*model.Event{{ID: "context", Kind: "state", Context: &model.Context{Operation: "compaction", Provenance: "source_native"}}},
	}})
	if view := viewer.View(); strings.Contains(view, "\x1b[") {
		t.Fatalf("NO_COLOR output contains ANSI escape codes: %q", view)
	}
}

func TestGalleryBrowseReadLandmarkAndFidelityFlow(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "tui.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := filepath.Join("..", "..", "examples", "gallery", "coding-agent-bugfix.ndjson")
	if _, err := app.IndexSource(context.Background(), store, source, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := Load(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	viewer := New(rows)
	viewer.color = false
	if view := viewer.View(); !strings.Contains(view, "coding-bugfix-rollout-01") || !strings.Contains(view, "attention ordered") {
		t.Fatalf("gallery Browse view is missing indexed data:\n%s", view)
	}

	updated, _ := viewer.Update(tea.KeyMsg{Type: tea.KeyEnter})
	read := updated.(Model)
	if read.mode != "read" || read.rows[read.selected].Events[read.event].Kind != "error" {
		t.Fatalf("Read did not land on first anomaly: mode=%s event=%d kind=%s", read.mode, read.event, read.rows[read.selected].Events[read.event].Kind)
	}
	firstError := read.event
	updated, _ = read.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	read = updated.(Model)
	if read.event <= firstError || read.rows[read.selected].Events[read.event].Kind != "error" {
		t.Fatalf("error landmark jump selected event=%d kind=%s after first=%d", read.event, read.rows[read.selected].Events[read.event].Kind, firstError)
	}
	updated, _ = read.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	read = updated.(Model)
	if read.fidelity != 4 || !strings.Contains(read.View(), "fidelity 4/5") {
		t.Fatalf("fidelity ladder did not reach previews-level rendering:\n%s", read.View())
	}
}
