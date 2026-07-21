package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

type Row struct {
	SourceID string
	Source   string
	Case     string
	Summary  rolloutindex.TrajectorySummary
	Events   []*model.Event
}

type Model struct {
	rows     []Row
	mode     string
	selected int
	event    int
	fidelity int
	depth    int
	width    int
	height   int
	color    bool
}

const (
	ansiReset = "\x1b[0m"
	ansiBlue  = "\x1b[34m"
	ansiRed   = "\x1b[31m"
	ansiInfra = "\x1b[33m"
	ansiGreen = "\x1b[32m"
)

func Load(ctx context.Context, store *rolloutindex.Index) ([]Row, error) {
	sources, err := store.Sources(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0)
	for _, source := range sources {
		groups, err := store.Groups(ctx, source.ID)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			summaries, err := store.GroupSummariesPage(ctx, source.ID, group.Value.ID, rolloutindex.MaxQueryRecords)
			if err != nil {
				return nil, err
			}
			for _, summary := range summaries.Items {
				page, err := store.Events(ctx, rolloutindex.EventQuery{SourceID: source.ID, TrajectoryID: summary.Trajectory.Value.ID, Limit: rolloutindex.MaxQueryRecords})
				if err != nil {
					return nil, err
				}
				if page.NextSequence != nil {
					return nil, fmt.Errorf("trajectory %q exceeds TUI event limit", summary.Trajectory.Value.ID)
				}
				trajectoryContext, err := store.TrajectoryContext(ctx, source.ID, summary.Trajectory.Value.ID)
				if err != nil {
					return nil, err
				}
				events := make([]*model.Event, 0, len(page.Events))
				for _, event := range page.Events {
					events = append(events, event.Value)
				}
				rows = append(rows, Row{SourceID: source.ID, Source: source.Path, Case: trajectoryContext.Case.Value.Name, Summary: summary, Events: events})
			}
		}
	}
	sort.SliceStable(rows, func(left, right int) bool { return attention(rows[left]) > attention(rows[right]) })
	return rows, nil
}

func attention(row Row) int {
	score := int(row.Summary.ErrorCount) * 100
	if row.Summary.Success != nil && !*row.Summary.Success {
		score += 60
	}
	if row.Summary.Reward != nil && *row.Summary.Reward < 1 {
		score += 30
	}
	return score
}

func New(rows []Row) Model {
	_, noColor := os.LookupEnv("NO_COLOR")
	return Model{rows: rows, mode: "browse", fidelity: 3, width: 80, height: 24, color: !noColor}
}
func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch value := message.(type) {
	case tea.WindowSizeMsg:
		if value.Width > 0 {
			model.width = value.Width
		}
		if value.Height > 0 {
			model.height = value.Height
		}
	case tea.KeyMsg:
		key := value.String()
		if key == "ctrl+c" || key == "q" {
			return model, func() tea.Msg { return tea.Quit() }
		}
		if key == "]" {
			model.fidelity = min(5, model.fidelity+1)
			return model, nil
		}
		if key == "[" {
			model.fidelity = max(0, model.fidelity-1)
			return model, nil
		}
		if model.mode == "browse" {
			return model.updateBrowse(key)
		}
		return model.updateRead(key)
	}
	return model, nil
}

func (model Model) updateBrowse(key string) (tea.Model, tea.Cmd) {
	if len(model.rows) == 0 {
		return model, nil
	}
	switch key {
	case "j", "down":
		model.selected = min(len(model.rows)-1, model.selected+1)
	case "k", "up":
		model.selected = max(0, model.selected-1)
	case "enter":
		model.mode = "read"
		model.event = firstAnomaly(model.rows[model.selected].Events)
	}
	return model, nil
}

func (model Model) updateRead(key string) (tea.Model, tea.Cmd) {
	events := model.rows[model.selected].Events
	switch key {
	case "j", "down":
		model.event = min(len(events)-1, model.event+1)
	case "k", "up":
		model.event = max(0, model.event-1)
	case "e":
		for offset := 1; offset <= len(events); offset++ {
			index := (model.event + offset) % len(events)
			if events[index].Kind == "error" {
				model.event = index
				break
			}
		}
	case "esc":
		if model.depth > 0 {
			model.depth--
		} else {
			model.mode = "browse"
		}
	case "enter":
		model.depth = min(3, model.depth+1)
	}
	return model, nil
}

func firstAnomaly(events []*model.Event) int {
	for index, event := range events {
		if event.Kind == "error" {
			return index
		}
	}
	return 0
}

func glyph(kind string) string {
	return map[string]string{"generation": "▸", "message": "‒", "observation": "·", "error": "✕", "tool": "▮", "environment_action": "▮", "reward": "◆", "grader": "◆", "state": "~", "log": "·", "artifact": "◆"}[kind]
}

func title(event *model.Event) string {
	if event.Metadata != nil {
		if value, ok := event.Metadata["title"].(string); ok && value != "" {
			return value
		}
	}
	if event.AlignmentKey != "" {
		return event.AlignmentKey
	}
	return strings.ReplaceAll(event.Kind, "_", " ")
}

func colorize(enabled bool, code, value string) string {
	if !enabled || code == "" || value == "" {
		return value
	}
	return code + value + ansiReset
}

func infrastructureText(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "infrastructure") || strings.Contains(value, "infra_") || strings.Contains(value, "environment")
}

func rowFailureColor(row Row) string {
	if infrastructureText(row.Summary.Termination) || infrastructureText(row.Summary.Trajectory.Value.Termination) {
		return ansiInfra
	}
	if raw, ok := row.Summary.Signals["failure_class"]; ok && infrastructureText(string(raw)) {
		return ansiInfra
	}
	return ansiRed
}

func verdict(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "pass", "passed", "true", "success", "successful", "accepted":
			return true, true
		case "fail", "failed", "false", "failure", "rejected":
			return false, true
		}
	case map[string]any:
		for _, key := range []string{"verdict", "pass", "passed", "success", "status", "result"} {
			if candidate, ok := typed[key]; ok {
				if passed, known := verdict(candidate); known {
					return passed, true
				}
			}
		}
	}
	return false, false
}

func eventColor(event *model.Event) string {
	if event.Context != nil || strings.HasPrefix(event.AlignmentKey, "context:") {
		return ansiBlue
	}
	if event.Kind == "error" {
		if infrastructureText(event.AlignmentKey + " " + title(event)) {
			return ansiInfra
		}
		return ansiRed
	}
	if event.Kind == "grader" {
		for _, candidate := range []any{event.Output, event.Data, event.Input, map[string]any(event.Metadata)} {
			if passed, known := verdict(candidate); known {
				if passed {
					return ansiGreen
				}
				return ansiRed
			}
		}
	}
	return ""
}

func (model Model) View() string {
	if model.mode == "read" && len(model.rows) > 0 {
		return model.readView()
	}
	return model.browseView()
}

func (model Model) browseView() string {
	lines := []string{"RLViz  Browse", fmt.Sprintf("%d trajectories · attention ordered · fidelity %d/5", len(model.rows), model.fidelity), strings.Repeat("─", min(model.width, 80))}
	available := max(1, model.height-6)
	start := max(0, min(model.selected-available/2, len(model.rows)-available))
	for index := start; index < min(len(model.rows), start+available); index++ {
		row := model.rows[index]
		cursor := "  "
		if index == model.selected {
			cursor = "> "
		}
		verdict := " "
		if row.Summary.ErrorCount > 0 || (row.Summary.Success != nil && !*row.Summary.Success) {
			verdict = "✕"
		}
		strip := caterpillar(row.Events, model.fidelity, 26, model.color)
		line := fmt.Sprintf("%s%s %-22s %-26s %3d", cursor, verdict, row.Summary.Trajectory.Value.ID, strip, len(row.Events))
		if verdict != " " {
			line = strings.Replace(line, cursor+verdict, cursor+colorize(model.color, rowFailureColor(row), verdict), 1)
		}
		lines = append(lines, truncate(line, model.width))
	}
	lines = append(lines, "", "j/k select  [/] fidelity  Enter read  q quit")
	return strings.Join(lines, "\n")
}

func caterpillar(events []*model.Event, fidelity, width int, color bool) string {
	if fidelity == 0 {
		return strings.Repeat("─", min(width, max(1, len(events)/2)))
	}
	var builder strings.Builder
	count := 0
	for _, event := range events {
		mark := glyph(event.Kind)
		if mark == "" {
			mark = "·"
		}
		builder.WriteString(colorize(color, eventColor(event), mark))
		count += utf8.RuneCountInString(mark)
		if count >= width {
			break
		}
	}
	return builder.String()
}

func (model Model) readView() string {
	row := model.rows[model.selected]
	events := row.Events
	if len(events) == 0 {
		return "RLViz  Read\nNo events\nEsc Browse  q quit"
	}
	current := events[model.event]
	episodes, retries, compactions := trajectoryShape(events)
	lines := []string{truncate(fmt.Sprintf("RLViz  Read  %s", row.Summary.Trajectory.Value.ID), model.width), truncate(fmt.Sprintf("%s · %s · event %d/%d · depth %d/3", row.Case, row.Summary.Trajectory.Value.Termination, model.event+1, len(events), model.depth), model.width), truncate("episodes: "+strings.Join(episodes, " → "), model.width), truncate(fmt.Sprintf("shape: %d errors/retries · %d compaction(s)", retries, compactions), model.width), strings.Repeat("─", min(model.width, 80)), truncate(caterpillar(events, max(3, model.fidelity), model.width-2, model.color), model.width), strings.Repeat("─", min(model.width, 80))}
	available := max(3, model.height-10)
	start := max(0, min(model.event-available/2, len(events)-available))
	for index := start; index < min(len(events), start+available); index++ {
		cursor := "  "
		if index == model.event {
			cursor = "> "
		}
		mark := glyph(events[index].Kind)
		line := truncate(fmt.Sprintf("%s%4d %s %-12s %s", cursor, events[index].Sequence, mark, events[index].Kind, title(events[index])), model.width)
		line = strings.Replace(line, " "+mark+" ", " "+colorize(model.color, eventColor(events[index]), mark)+" ", 1)
		lines = append(lines, line)
	}
	lines = append(lines, strings.Repeat("─", min(model.width, 80)), truncate(fmt.Sprintf("selected #%d · %s · fidelity %d/5", current.Sequence, title(current), model.fidelity), model.width), "j/k events  e error  [/] fidelity  Enter/Esc depth  q quit")
	if len(lines) > model.height {
		lines = lines[:model.height]
	}
	return strings.Join(lines, "\n")
}

func trajectoryShape(events []*model.Event) ([]string, int, int) {
	episodes := make([]string, 0)
	seen := make(map[string]bool)
	retries, compactions := 0, 0
	for _, event := range events {
		if strings.HasPrefix(event.AlignmentKey, "episode:") {
			name := strings.TrimPrefix(event.AlignmentKey, "episode:")
			if name != "" && !seen[name] {
				seen[name] = true
				episodes = append(episodes, name)
			}
		}
		if event.Kind == "error" {
			retries++
		}
		if event.Context != nil && event.Context.Operation == "compaction" {
			compactions++
		}
	}
	if len(episodes) == 0 {
		episodes = append(episodes, "outcome")
	}
	return episodes, retries, compactions
}

func truncate(value string, width int) string {
	if width <= 0 || utf8.RuneCountInString(value) <= width {
		return value
	}
	runes := []rune(value)
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "…"
}
