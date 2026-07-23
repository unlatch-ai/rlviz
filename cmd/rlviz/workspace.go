package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/daemon"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type repeatedString []string

func (values *repeatedString) String() string { return strings.Join(*values, ",") }
func (values *repeatedString) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

type workspaceResult struct {
	WorkspaceID string           `json:"workspace_id"`
	Revision    int64            `json:"revision"`
	URL         string           `json:"url,omitempty"`
	Path        string           `json:"path,omitempty"`
	SourceID    string           `json:"source_id,omitempty"`
	Workspace   server.Workspace `json:"workspace"`
}

func runWorkspace(arguments []string) {
	if len(arguments) == 0 {
		printWorkspaceHelp()
		return
	}
	switch arguments[0] {
	case "open":
		runWorkspaceOpen(arguments[1:])
	case "show":
		runWorkspaceShow(arguments[1:])
	case "add":
		runWorkspaceAdd(arguments[1:], false)
	case "detail":
		runWorkspaceAdd(arguments[1:], true)
	case "group":
		runWorkspaceGroup(arguments[1:])
	case "help", "-h", "--help":
		printWorkspaceHelp()
	default:
		fatalError("workspace", false, fmt.Errorf("unknown workspace command %q", arguments[0]))
	}
}

func runWorkspaceOpen(arguments []string) {
	flags := flag.NewFlagSet("workspace open", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	noOpen := flags.Bool("no-open", false, "do not open the browser")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	group := flags.String("group", "rollouts", "collection grouping: rollouts or trials")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	presentationPath := flags.String("presentation", "", "validated declarative presentation JSON")
	var trajectories, details repeatedString
	flags.Var(&trajectories, "trajectory", "trajectory ID to open; repeatable")
	flags.Var(&details, "detail", "trajectory ID with a pinned detail module; repeatable")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rlviz workspace open [--trajectory ID]... [--detail ID]... [--group rollouts|trials] [--no-open] [--json] SOURCE")
	}
	if err := flags.Parse(normalizeSubcommandArguments(arguments, map[string]bool{"--group": true, "--adapter": true, "--presentation": true, "--trajectory": true, "--detail": true})); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}
	if *group != "rollouts" && *group != "trials" {
		fatalError("workspace open", *jsonOutput, errors.New("--group must be rollouts or trials"))
	}
	presentation, err := loadPresentationFile(*presentationPath)
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ensured, err := ensureDaemon(ctx)
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	client := daemon.Client{}
	registered, err := client.Register(ctx, ensured.Metadata, daemon.RegisterRequest{Path: flags.Arg(0), Adapter: *adapter, Presentation: presentation})
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	ids := uniqueStrings(append(append([]string{}, trajectories...), details...))
	if err := validateTrajectoryIDs(ctx, registered.SourceID, ids); err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	workspace := newWorkspace(registered.SourceID, ids, details, *group)
	created, err := client.CreateWorkspace(ctx, ensured.Metadata, workspace)
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	if err := json.Unmarshal(created.Workspace, &workspace); err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	viewerURL, err := resolveViewerURL(ensured.Metadata, registered.URL)
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	viewerURL, err = withWorkspaceID(viewerURL, created.ID)
	if err != nil {
		fatalError("workspace open", *jsonOutput, err)
	}
	result := workspaceResult{WorkspaceID: created.ID, Revision: created.Revision, URL: viewerURL, Path: registered.Path, SourceID: registered.SourceID, Workspace: workspace}
	writeOutput(result, *jsonOutput, fmt.Sprintf("Opened workspace %s at %s", created.ID, viewerURL))
	if !*noOpen {
		if err := openBrowser(viewerURL); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
}

func runWorkspaceShow(arguments []string) {
	flags := flag.NewFlagSet("workspace show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	if err := flags.Parse(normalizeSubcommandArguments(arguments, nil)); err != nil || flags.NArg() != 1 {
		fmt.Fprintln(flags.Output(), "Usage: rlviz workspace show [--json] WORKSPACE_ID")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	metadata, err := liveDaemon(ctx)
	if err != nil {
		fatalError("workspace show", *jsonOutput, err)
	}
	response, err := (daemon.Client{}).Workspace(ctx, metadata, flags.Arg(0))
	if err != nil {
		fatalError("workspace show", *jsonOutput, err)
	}
	var workspace server.Workspace
	if err := json.Unmarshal(response.Workspace, &workspace); err != nil {
		fatalError("workspace show", *jsonOutput, err)
	}
	result := workspaceResult{WorkspaceID: response.ID, Revision: response.Revision, Workspace: workspace}
	writeOutput(result, *jsonOutput, fmt.Sprintf("Workspace %s revision %d · %d trajectories · %s grouping", response.ID, response.Revision, len(workspace.Lanes), workspace.CollectionView))
}

func runWorkspaceAdd(arguments []string, pinDetail bool) {
	command := "workspace add"
	if pinDetail {
		command = "workspace detail"
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	trajectory := flags.String("trajectory", "", "trajectory ID")
	if err := flags.Parse(normalizeSubcommandArguments(arguments, map[string]bool{"--trajectory": true})); err != nil || flags.NArg() != 1 || *trajectory == "" {
		fmt.Fprintf(flags.Output(), "Usage: rlviz %s [--json] --trajectory ID WORKSPACE_ID\n", command)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	metadata, err := liveDaemon(ctx)
	if err != nil {
		fatalError(command, *jsonOutput, err)
	}
	client := daemon.Client{}
	response, workspace, err := getWorkspace(ctx, client, metadata, flags.Arg(0))
	if err != nil {
		fatalError(command, *jsonOutput, err)
	}
	sourceID := workspaceSourceID(workspace)
	if sourceID == "" {
		fatalError(command, *jsonOutput, errors.New("workspace has no source; open a source-backed workspace first"))
	}
	if err := validateTrajectoryIDs(ctx, sourceID, []string{*trajectory}); err != nil {
		fatalError(command, *jsonOutput, err)
	}
	id := workspaceLaneID(sourceID, *trajectory)
	found := false
	for _, lane := range workspace.Lanes {
		found = found || lane.ID == id
	}
	if !found {
		band := "focus"
		focus := 0
		for _, lane := range workspace.Lanes {
			if lane.Band == "focus" {
				focus++
			}
		}
		if focus >= 2 {
			band = "context"
		}
		workspace.Lanes = append(workspace.Lanes, server.WorkspaceLane{ID: id, SourceID: sourceID, TrajectoryID: *trajectory, Band: band, Depth: 1, Fidelity: 1, Axis: server.WorkspaceAxis{End: 1}, DescentStack: []any{}})
	}
	workspace.Active = id
	if pinDetail && !contains(workspace.Details, id) {
		workspace.Details = append(workspace.Details, id)
		workspace.Active = "detail:" + id
	}
	updated, err := client.ReplaceWorkspace(ctx, metadata, response.ID, workspace)
	if err != nil {
		fatalError(command, *jsonOutput, err)
	}
	if err := json.Unmarshal(updated.Workspace, &workspace); err != nil {
		fatalError(command, *jsonOutput, err)
	}
	result := workspaceResult{WorkspaceID: updated.ID, Revision: updated.Revision, SourceID: sourceID, Workspace: workspace}
	writeOutput(result, *jsonOutput, fmt.Sprintf("Updated workspace %s to revision %d", updated.ID, updated.Revision))
}

func runWorkspaceGroup(arguments []string) {
	flags := flag.NewFlagSet("workspace group", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	by := flags.String("by", "", "rollouts or trials")
	if err := flags.Parse(normalizeSubcommandArguments(arguments, map[string]bool{"--by": true})); err != nil || flags.NArg() != 1 || (*by != "rollouts" && *by != "trials") {
		fmt.Fprintln(flags.Output(), "Usage: rlviz workspace group [--json] --by rollouts|trials WORKSPACE_ID")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	metadata, err := liveDaemon(ctx)
	if err != nil {
		fatalError("workspace group", *jsonOutput, err)
	}
	client := daemon.Client{}
	response, workspace, err := getWorkspace(ctx, client, metadata, flags.Arg(0))
	if err != nil {
		fatalError("workspace group", *jsonOutput, err)
	}
	workspace.CollectionView = *by
	updated, err := client.ReplaceWorkspace(ctx, metadata, response.ID, workspace)
	if err != nil {
		fatalError("workspace group", *jsonOutput, err)
	}
	if err := json.Unmarshal(updated.Workspace, &workspace); err != nil {
		fatalError("workspace group", *jsonOutput, err)
	}
	result := workspaceResult{WorkspaceID: updated.ID, Revision: updated.Revision, SourceID: workspaceSourceID(workspace), Workspace: workspace}
	writeOutput(result, *jsonOutput, fmt.Sprintf("Workspace %s now groups by %s", updated.ID, *by))
}

func ensureDaemon(ctx context.Context) (daemon.EnsureResult, error) {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		return daemon.EnsureResult{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return daemon.EnsureResult{}, fmt.Errorf("locate rlviz executable: %w", err)
	}
	manager := daemon.Manager{Paths: paths, Executable: executable, Args: []string{"daemon", "serve", "--runtime-dir", paths.RuntimeDir}, Version: version}
	return manager.Ensure(ctx)
}

func liveDaemon(ctx context.Context) (daemon.Metadata, error) {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		return daemon.Metadata{}, err
	}
	return daemon.LoadLiveMetadata(ctx, paths, daemon.Client{})
}

func getWorkspace(ctx context.Context, client daemon.Client, metadata daemon.Metadata, id string) (daemon.WorkspaceResponse, server.Workspace, error) {
	response, err := client.Workspace(ctx, metadata, id)
	if err != nil {
		return daemon.WorkspaceResponse{}, server.Workspace{}, err
	}
	var workspace server.Workspace
	if err := json.Unmarshal(response.Workspace, &workspace); err != nil {
		return daemon.WorkspaceResponse{}, server.Workspace{}, err
	}
	return response, workspace, nil
}

func newWorkspace(sourceID string, trajectories, details []string, grouping string) server.Workspace {
	workspace := server.Workspace{Version: 3, RailExpanded: true, CollectionView: grouping, GuideOpen: len(trajectories) == 0, SettingsOpen: true, DetailOpen: len(trajectories) > 0, Direction: "rows", Active: "rail", Lanes: []server.WorkspaceLane{}, Details: []string{}}
	for index, trajectoryID := range trajectories {
		band := "focus"
		if index >= 2 {
			band = "context"
		}
		id := workspaceLaneID(sourceID, trajectoryID)
		workspace.Lanes = append(workspace.Lanes, server.WorkspaceLane{ID: id, SourceID: sourceID, TrajectoryID: trajectoryID, Band: band, Depth: 1, Fidelity: 1, Axis: server.WorkspaceAxis{End: 1}, DescentStack: []any{}})
		if contains(details, trajectoryID) {
			workspace.Details = append(workspace.Details, id)
		}
	}
	if len(workspace.Lanes) > 0 {
		workspace.Active = workspace.Lanes[0].ID
	}
	return workspace
}

func validateTrajectoryIDs(ctx context.Context, sourceID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	paths, err := daemon.DefaultPaths()
	if err != nil {
		return err
	}
	store, err := rolloutindex.Open(paths.IndexFile)
	if err != nil {
		return err
	}
	defer store.Close()
	for _, id := range ids {
		if _, err := store.TrajectoryContext(ctx, sourceID, id); err != nil {
			return fmt.Errorf("trajectory %q is not indexed for source %s: %w", id, sourceID, err)
		}
	}
	return nil
}

func workspaceSourceID(workspace server.Workspace) string {
	if len(workspace.Lanes) == 0 {
		return ""
	}
	return workspace.Lanes[0].SourceID
}

func workspaceLaneID(sourceID, trajectoryID string) string {
	return url.PathEscape(sourceID) + ":" + url.PathEscape(trajectoryID)
}

func withWorkspaceID(value, id string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse viewer URL: %w", err)
	}
	query := parsed.Query()
	query.Set("workspace_id", id)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeSubcommandArguments(arguments []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(arguments))
	positions := make([]string, 0, 1)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--json" || argument == "--no-open" || argument == "--failed" || argument == "--errors" {
			flags = append(flags, argument)
			continue
		}
		if valueFlags[argument] {
			flags = append(flags, argument)
			if index+1 < len(arguments) {
				index++
				flags = append(flags, arguments[index])
			}
			continue
		}
		matched := false
		for name := range valueFlags {
			if strings.HasPrefix(argument, name+"=") {
				flags = append(flags, argument)
				matched = true
				break
			}
		}
		if !matched {
			positions = append(positions, argument)
		}
	}
	return append(flags, positions...)
}

func printWorkspaceHelp() {
	fmt.Print(`RLViz workspaces

Usage:
  rlviz workspace open SOURCE [--trajectory ID]... [--detail ID]... [--group rollouts|trials] [--no-open] [--json]
  rlviz workspace show WORKSPACE_ID [--json]
  rlviz workspace add WORKSPACE_ID --trajectory ID [--json]
  rlviz workspace detail WORKSPACE_ID --trajectory ID [--json]
  rlviz workspace group WORKSPACE_ID --by rollouts|trials [--json]
`)
}

type trajectoryQueryRow struct {
	SourceID     string                     `json:"source_id"`
	GroupID      string                     `json:"group_id"`
	Run          string                     `json:"run,omitempty"`
	Trial        string                     `json:"trial,omitempty"`
	Group        string                     `json:"group,omitempty"`
	TrajectoryID string                     `json:"trajectory_id"`
	Summary      map[string]any             `json:"summary"`
	Signals      map[string]json.RawMessage `json:"signals,omitempty"`
}

type trajectoryQueryBucket struct {
	Key          string               `json:"key"`
	Trajectories []trajectoryQueryRow `json:"trajectories"`
}

func runTrajectories(arguments []string) {
	flags := flag.NewFlagSet("trajectories", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	query := flags.String("query", "", "case-insensitive ID and name search")
	failed := flags.Bool("failed", false, "only failed trajectories")
	errorsOnly := flags.Bool("errors", false, "only trajectories containing errors")
	groupBy := flags.String("group-by", "rollout", "rollout or trial")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rlviz trajectories [--query TEXT] [--failed] [--errors] [--group-by rollout|trial] [--adapter PATH] [--json] SOURCE")
	}
	if err := flags.Parse(normalizeSubcommandArguments(arguments, map[string]bool{"--query": true, "--group-by": true, "--adapter": true})); err != nil || flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}
	if *groupBy != "rollout" && *groupBy != "trial" {
		fatalError("trajectories", *jsonOutput, errors.New("--group-by must be rollout or trial"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ensured, err := ensureDaemon(ctx)
	if err != nil {
		fatalError("trajectories", *jsonOutput, err)
	}
	registered, err := (daemon.Client{}).Register(ctx, ensured.Metadata, daemon.RegisterRequest{Path: flags.Arg(0), Adapter: *adapter})
	if err != nil {
		fatalError("trajectories", *jsonOutput, err)
	}
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("trajectories", *jsonOutput, err)
	}
	store, err := rolloutindex.Open(paths.IndexFile)
	if err != nil {
		fatalError("trajectories", *jsonOutput, err)
	}
	defer store.Close()
	groups, err := store.Groups(ctx, registered.SourceID)
	if err != nil {
		fatalError("trajectories", *jsonOutput, err)
	}
	needle := strings.ToLower(strings.TrimSpace(*query))
	rows := make([]trajectoryQueryRow, 0)
	for _, group := range groups {
		page, err := store.GroupSummariesPage(ctx, registered.SourceID, group.Value.ID, rolloutindex.MaxQueryRecords)
		if err != nil {
			fatalError("trajectories", *jsonOutput, err)
		}
		if page.Total > int64(len(page.Items)) {
			fatalError("trajectories", *jsonOutput, fmt.Errorf("group %q exceeds %d trajectory query limit", group.Value.ID, rolloutindex.MaxQueryRecords))
		}
		for _, summary := range page.Items {
			search := strings.ToLower(strings.Join([]string{summary.Trajectory.Value.ID, summary.RunName, summary.CaseName, summary.GroupName, summary.Status, summary.Termination}, " "))
			if needle != "" && !strings.Contains(search, needle) {
				continue
			}
			if *failed && !failedTrajectory(summary.Success, summary.Status, summary.Termination) {
				continue
			}
			if *errorsOnly && summary.ErrorCount == 0 {
				continue
			}
			rows = append(rows, trajectoryQueryRow{SourceID: registered.SourceID, GroupID: group.Value.ID, Run: summary.RunName, Trial: summary.CaseName, Group: summary.GroupName, TrajectoryID: summary.Trajectory.Value.ID, Signals: summary.Signals, Summary: map[string]any{"success": summary.Success, "reward": summary.Reward, "status": summary.Status, "termination": summary.Termination, "events": summary.EventCount, "errors": summary.ErrorCount, "tokens": summary.TokenCount, "latency_ms": summary.LatencyMS}})
		}
	}
	bucketsByKey := make(map[string][]trajectoryQueryRow)
	for _, row := range rows {
		key := row.Group
		if *groupBy == "trial" {
			key = row.Trial
		}
		bucketsByKey[key] = append(bucketsByKey[key], row)
	}
	keys := make([]string, 0, len(bucketsByKey))
	for key := range bucketsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buckets := make([]trajectoryQueryBucket, 0, len(keys))
	for _, key := range keys {
		buckets = append(buckets, trajectoryQueryBucket{Key: key, Trajectories: bucketsByKey[key]})
	}
	result := map[string]any{"source_id": registered.SourceID, "path": filepath.Clean(registered.Path), "group_by": *groupBy, "query": *query, "count": len(rows), "groups": buckets}
	writeOutput(result, *jsonOutput, fmt.Sprintf("Found %d trajectories in %d %s groups", len(rows), len(buckets), *groupBy))
}

func failedTrajectory(success *bool, status, termination string) bool {
	if success != nil {
		return !*success
	}
	value := strings.ToLower(status + " " + termination)
	return strings.Contains(value, "fail") || strings.Contains(value, "error") || strings.Contains(value, "violation") || strings.Contains(value, "cancel") || strings.Contains(value, "timeout")
}
