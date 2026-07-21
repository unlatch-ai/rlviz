package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

const maxBrowseRows = 1000

type indexedBrowseReader interface {
	Sources(context.Context) ([]rolloutindex.SourceInfo, error)
	Groups(context.Context, string) ([]rolloutindex.IndexedRecord[*model.Group], error)
}

type browseRow struct {
	SourceID   string            `json:"source_id"`
	SourceName string            `json:"source_name"`
	RunName    string            `json:"run_name,omitempty"`
	CaseName   string            `json:"case_name,omitempty"`
	GroupName  string            `json:"group_name,omitempty"`
	Trajectory *model.Trajectory `json:"trajectory"`
	Metrics    browseMetrics     `json:"metrics"`
}

type browseMetrics struct {
	Signals       map[string]json.RawMessage `json:"signals,omitempty"`
	Reward        *float64                   `json:"reward,omitempty"`
	Success       *bool                      `json:"success,omitempty"`
	TokenCount    *int64                     `json:"token_count,omitempty"`
	LatencyMS     *float64                   `json:"latency_ms,omitempty"`
	Status        string                     `json:"status,omitempty"`
	Termination   string                     `json:"termination,omitempty"`
	EventCount    int64                      `json:"event_count"`
	ErrorCount    int64                      `json:"error_count"`
	FirstSequence *int64                     `json:"first_sequence,omitempty"`
	LastSequence  *int64                     `json:"last_sequence,omitempty"`
	SignalCount   int64                      `json:"signal_count"`
	ArtifactCount int64                      `json:"artifact_count"`
}

func browseSummary(summary rolloutindex.TrajectorySummary) browseMetrics {
	return browseMetrics{
		Signals: summary.Signals, Reward: summary.Reward, Success: summary.Success,
		TokenCount: summary.TokenCount, LatencyMS: summary.LatencyMS, Status: summary.Status,
		Termination: summary.Termination, EventCount: summary.EventCount, ErrorCount: summary.ErrorCount,
		FirstSequence: summary.FirstSequence, LastSequence: summary.LastSequence,
		SignalCount: summary.SignalCount, ArtifactCount: summary.ArtifactCount,
	}
}

func (api *indexedAPI) browse(response http.ResponseWriter, request *http.Request) {
	if err := validateQuery(request.URL.Query(), map[string]bool{}); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	reader, ok := api.reader.(indexedBrowseReader)
	if !ok {
		writeJSONError(response, http.StatusNotImplemented, "browse_unavailable", errors.New("browse index is unavailable"))
		return
	}
	sources, err := reader.Sources(request.Context())
	if err != nil {
		api.writeReadError(response, "browse_failed", err)
		return
	}
	rows := make([]browseRow, 0)
	for _, source := range sources {
		groups, err := reader.Groups(request.Context(), source.ID)
		if err != nil {
			api.writeReadError(response, "browse_failed", err)
			return
		}
		for _, group := range groups {
			remaining := maxBrowseRows - len(rows)
			limit := remaining
			if limit == 0 {
				limit = 1
			}
			page, err := api.reader.GroupSummariesPage(request.Context(), source.ID, group.Value.ID, limit)
			if err != nil {
				api.writeReadError(response, "browse_failed", err)
				return
			}
			if (remaining == 0 && page.Total > 0) || page.Total > int64(remaining) {
				writeJSONError(response, http.StatusRequestEntityTooLarge, "browse_too_large", errors.New("browse collection exceeds 1000 trajectories"))
				return
			}
			for _, summary := range page.Items {
				rows = append(rows, browseRow{
					SourceID: source.ID, SourceName: filepath.Base(source.Path),
					RunName: summary.RunName, CaseName: summary.CaseName,
					GroupName: summary.GroupName, Trajectory: summary.Trajectory.Value, Metrics: browseSummary(summary),
				})
			}
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{"sources": sources, "trajectories": rows, "count": len(rows)})
}
