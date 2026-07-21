package server

import (
	"context"
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
	SourceID   string                         `json:"source_id"`
	SourceName string                         `json:"source_name"`
	RunName    string                         `json:"run_name,omitempty"`
	CaseName   string                         `json:"case_name,omitempty"`
	GroupName  string                         `json:"group_name,omitempty"`
	Trajectory *model.Trajectory              `json:"trajectory"`
	Metrics    rolloutindex.TrajectorySummary `json:"metrics"`
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
			if remaining <= 0 {
				writeJSONError(response, http.StatusRequestEntityTooLarge, "browse_too_large", errors.New("browse collection exceeds 1000 trajectories"))
				return
			}
			page, err := api.reader.GroupSummariesPage(request.Context(), source.ID, group.Value.ID, remaining)
			if err != nil {
				api.writeReadError(response, "browse_failed", err)
				return
			}
			if page.Total > int64(len(page.Items)) {
				writeJSONError(response, http.StatusRequestEntityTooLarge, "browse_too_large", errors.New("browse collection exceeds 1000 trajectories"))
				return
			}
			for _, summary := range page.Items {
				ctx, err := api.reader.TrajectoryContext(request.Context(), source.ID, summary.Trajectory.Value.ID)
				if err != nil {
					api.writeReadError(response, "browse_failed", err)
					return
				}
				rows = append(rows, browseRow{
					SourceID: source.ID, SourceName: filepath.Base(source.Path),
					RunName: ctx.Run.Value.Name, CaseName: ctx.Case.Value.Name,
					GroupName: ctx.Group.Value.Name, Trajectory: summary.Trajectory.Value, Metrics: summary,
				})
			}
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{"sources": sources, "trajectories": rows, "count": len(rows)})
}
