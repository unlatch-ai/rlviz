package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/plugins"
)

type formatInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Kind         string   `json:"kind"`
	APIVersion   string   `json:"api_version"`
	Version      string   `json:"version,omitempty"`
	Status       string   `json:"status"`
	Path         string   `json:"path,omitempty"`
	Digest       string   `json:"digest,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Description  string   `json:"description,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type formatsResult struct {
	Formats []formatInfo `json:"formats"`
}

func runFormats(arguments []string) {
	flags := flag.NewFlagSet("formats", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 0 {
		fmt.Fprintln(flags.Output(), "Usage: rlviz formats [--json]")
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
	result := collectFormats(entries)
	writeOutput(result, *jsonOutput, formatListText(result.Formats))
}

func collectFormats(entries []plugins.TrustEntry) formatsResult {
	formats := []formatInfo{{
		ID: "canonical-ndjson", Name: "Canonical NDJSON", Source: "built_in",
		Kind: "Adapter", APIVersion: model.APIVersion, Status: "available",
		Capabilities: []string{"adapter.stream", "groups", "artifacts", "source-provenance"},
		Description:  "Versioned newline-delimited RLViz canonical records",
	}}
	for _, entry := range entries {
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
	}
	return formatsResult{Formats: formats}
}

func formatListText(formats []formatInfo) string {
	lines := []string{"Built in:"}
	for _, format := range formats {
		if format.Source == "built_in" {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", format.ID, format.APIVersion, format.Status))
		}
	}
	trusted := make([]string, 0)
	for _, format := range formats {
		if format.Source != "built_in" {
			trusted = append(trusted, fmt.Sprintf("  %s  %s  %s", format.ID, format.Kind, format.Status))
		}
	}
	if len(trusted) == 0 {
		lines = append(lines, "", "Trusted plugins:", "  none")
	} else {
		lines = append(lines, "", "Trusted plugins:")
		lines = append(lines, trusted...)
	}
	lines = append(lines, "", "Example adapters are not built-in formats. See docs/supported-formats.md.")
	return strings.Join(lines, "\n")
}
