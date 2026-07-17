package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/plugins"
)

type formatInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	Kind            string   `json:"kind"`
	APIVersion      string   `json:"api_version"`
	Version         string   `json:"version,omitempty"`
	Status          string   `json:"status"`
	Path            string   `json:"path,omitempty"`
	Digest          string   `json:"digest,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Description     string   `json:"description,omitempty"`
	Error           string   `json:"error,omitempty"`
	Rank            int      `json:"rank,omitempty"`
	DiscoverySource string   `json:"discovery_source,omitempty"`
}

type formatsResult struct {
	SchemaVersion   int                      `json:"schema_version"`
	Formats         []formatInfo             `json:"formats"`
	DiscoveryIssues []plugins.DiscoveryIssue `json:"discovery_issues"`
}

type repeatedPaths []string

func (paths *repeatedPaths) String() string { return strings.Join(*paths, ",") }
func (paths *repeatedPaths) Set(value string) error {
	*paths = append(*paths, value)
	return nil
}

func runFormats(arguments []string) {
	flags := flag.NewFlagSet("formats", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	project := flags.String("project", ".", "project directory whose .rlviz/plugins directory may be inventoried")
	var extraRoots repeatedPaths
	flags.Var(&extraRoots, "plugin-root", "additional explicit plugin directory to inventory (repeatable)")
	_ = flags.Parse(arguments)
	if flags.NArg() != 0 {
		fmt.Fprintln(flags.Output(), "Usage: rlviz formats [--json] [--project DIR] [--plugin-root DIR]...")
		os.Exit(2)
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("formats", *jsonOutput, err)
	}
	entries, err := store.List()
	if err != nil {
		fatalError("formats", *jsonOutput, err)
	}
	roots := make([]plugins.DiscoveryRoot, 0, len(extraRoots)+2)
	for _, root := range extraRoots {
		roots = append(roots, plugins.DiscoveryRoot{Path: root, Source: "explicit"})
	}
	roots = append(roots,
		plugins.DiscoveryRoot{Path: filepath.Join(*project, ".rlviz", "plugins"), Source: "project"},
		plugins.DiscoveryRoot{Path: filepath.Join(filepath.Dir(store.Path), "plugins"), Source: "user"},
	)
	discovery := plugins.Discover(plugins.DiscoveryOptions{Roots: roots, TrustEntries: entries})
	result := collectFormats(entries, discovery)
	writeOutput(result, *jsonOutput, formatListText(result.Formats))
}

func collectFormats(entries []plugins.TrustEntry, discoveries ...plugins.DiscoveryResult) formatsResult {
	formats := []formatInfo{{
		ID: "canonical-ndjson", Name: "Canonical NDJSON", Source: "built_in",
		Kind: "Adapter", APIVersion: model.APIVersion, Status: "available",
		Capabilities: []string{"adapter.stream", "groups", "artifacts", "source-provenance"},
		Description:  "Versioned newline-delimited RLViz canonical records",
	}}
	knownPaths := map[string]int{}
	for _, entry := range entries {
		pathKey := cleanFormatPath(entry.Path)
		info := formatInfo{
			ID: entry.Path, Name: entry.Path, Source: "trusted_plugin", Kind: "Plugin",
			Status: "unavailable", Path: entry.Path, Digest: entry.Digest,
		}
		plugin, err := plugins.Load(entry.Path)
		if err != nil {
			info.Error = err.Error()
			formats = append(formats, info)
			continue
		}
		info.ID = plugin.Manifest.Name
		info.Name = plugin.Manifest.Name
		info.Kind = plugin.Manifest.Kind
		info.APIVersion = plugin.Manifest.APIVersion
		info.Version = plugin.Manifest.Version
		info.Capabilities = plugin.Manifest.Capabilities
		info.Description = plugin.Manifest.Description
		if plugin.Digest == entry.Digest {
			info.Status = "trusted"
		} else {
			info.Status = "changed"
			info.Error = "plugin contents changed since trust was granted"
		}
		formats = append(formats, info)
		knownPaths[pathKey] = len(formats) - 1
	}
	issues := []plugins.DiscoveryIssue{}
	for _, discovery := range discoveries {
		issues = append(issues, discovery.Issues...)
		for _, item := range discovery.Plugins {
			if index, exists := knownPaths[cleanFormatPath(item.Path)]; exists {
				formats[index].Rank = item.Rank
				formats[index].DiscoverySource = item.Source
				continue
			}
			formats = append(formats, formatInfo{
				ID: item.Name, Name: item.Name, Source: item.Source + "_plugin", Kind: item.Kind,
				APIVersion: item.APIVersion, Version: item.Version, Status: item.Status,
				Path: item.Path, Digest: item.Digest, Capabilities: item.Capabilities,
				Description: item.Description, Error: item.Error, Rank: item.Rank,
				DiscoverySource: item.Source,
			})
			knownPaths[cleanFormatPath(item.Path)] = len(formats) - 1
		}
	}
	formats = append(formats, documentedExampleFormats()...)
	return formatsResult{SchemaVersion: 1, Formats: formats, DiscoveryIssues: issues}
}

func documentedExampleFormats() []formatInfo {
	return []formatInfo{
		{ID: "simple-agent-jsonl", Name: "Simple JSONL", Source: "example_adapter", Kind: "Adapter", APIVersion: model.APIVersion, Version: "0.1.0", Status: "example", Capabilities: []string{"adapter.probe", "adapter.stream"}, Description: "Small non-canonical JSONL reference adapter"},
		{ID: "inspect-ai-eval-log-json-v2", Name: "Inspect AI EvalLog JSON v2", Source: "example_adapter", Kind: "Adapter", APIVersion: model.APIVersion, Version: "0.1.0", Status: "example", Capabilities: []string{"adapter.probe", "adapter.stream"}, Description: "Model, tool, score, and compaction events from EvalLog JSON"},
		{ID: "prime-verifiers-generate-outputs", Name: "Prime Intellect Verifiers GenerateOutputs", Source: "example_adapter", Kind: "Adapter", APIVersion: model.APIVersion, Version: "0.1.0", Status: "example", Capabilities: []string{"adapter.probe", "adapter.stream"}, Description: "RL rollout steps, token masks, rewards, and metrics"},
	}
}

func cleanFormatPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

func formatListText(formats []formatInfo) string {
	lines := []string{"Built in:"}
	for _, format := range formats {
		if format.Source == "built_in" {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", format.ID, format.APIVersion, format.Status))
		}
	}
	trusted := make([]string, 0)
	examples := make([]string, 0)
	discovered := make([]string, 0)
	for _, format := range formats {
		if format.Source == "trusted_plugin" {
			trusted = append(trusted, fmt.Sprintf("  %s  %s  %s", format.ID, format.Kind, format.Status))
		} else if format.Source == "example_adapter" {
			examples = append(examples, fmt.Sprintf("  %s  %s", format.ID, format.Status))
		} else if format.Source != "built_in" {
			name := format.ID
			if name == "" {
				name = filepath.Base(format.Path)
			}
			discovered = append(discovered, fmt.Sprintf("  %d  %s  %s  %s", format.Rank, name, format.Source, format.Status))
		}
	}
	if len(trusted) == 0 {
		lines = append(lines, "", "Trusted plugins:", "  none")
	} else {
		lines = append(lines, "", "Trusted plugins:")
		lines = append(lines, trusted...)
	}
	lines = append(lines, "", "Example adapters (explicit selection and trust required):")
	lines = append(lines, examples...)
	if len(discovered) == 0 {
		lines = append(lines, "", "Discovered plugin manifests:", "  none")
	} else {
		lines = append(lines, "", "Discovered plugin manifests (inventory only; not executed):")
		lines = append(lines, discovered...)
	}
	lines = append(lines, "", "See docs/supported-formats.md for example paths and exact support boundaries.")
	return strings.Join(lines, "\n")
}
