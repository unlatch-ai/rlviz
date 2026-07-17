package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	fixturedata "github.com/unlatch-ai/rlviz/fixtures"
	"github.com/unlatch-ai/rlviz/internal/daemon"
)

const demoFilename = "demo-v1alpha1.ndjson"

func runDemo(arguments []string) {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	noOpen := flags.Bool("no-open", false, "do not open the browser")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	flags.Usage = func() { fmt.Fprintln(flags.Output(), "Usage: rlviz demo [--no-open] [--json]") }
	if err := flags.Parse(arguments); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("demo", *jsonOutput, err)
	}
	path, err := ensureDemoSource(paths)
	if err != nil {
		fatalError("demo", *jsonOutput, err)
	}
	openSource(path, "", nil, *noOpen, *jsonOutput, "demo")
}

func ensureDemoSource(paths daemon.Paths) (string, error) {
	if err := paths.EnsureRuntimeDir(); err != nil {
		return "", err
	}
	path := filepath.Join(paths.RuntimeDir, demoFilename)
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, fixturedata.DemoNDJSON) {
		if err := os.Chmod(path, 0o600); err != nil {
			return "", fmt.Errorf("secure demo fixture: %w", err)
		}
		return path, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read demo fixture: %w", err)
	}
	temporary, err := os.CreateTemp(paths.RuntimeDir, ".demo-*.ndjson")
	if err != nil {
		return "", fmt.Errorf("create demo fixture: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(fixturedata.DemoNDJSON); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, path); err != nil {
		return "", fmt.Errorf("install demo fixture: %w", err)
	}
	return path, nil
}

func openSource(path, adapter string, presentationConfig json.RawMessage, noOpen, jsonOutput bool, command string) {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError(command, jsonOutput, err)
	}
	executable, err := os.Executable()
	if err != nil {
		fatalError(command, jsonOutput, fmt.Errorf("locate rlviz executable: %w", err))
	}
	manager := daemon.Manager{
		Paths: paths, Executable: executable,
		Args: []string{"daemon", "serve", "--runtime-dir", paths.RuntimeDir}, Version: version,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensured, err := manager.Ensure(ctx)
	if err != nil {
		fatalError(command, jsonOutput, err)
	}
	registered, err := (daemon.Client{}).Register(ctx, ensured.Metadata, daemon.RegisterRequest{Path: path, Adapter: adapter, Presentation: presentationConfig})
	if err != nil {
		fatalError(command, jsonOutput, err)
	}
	viewerURL, err := resolveViewerURL(ensured.Metadata, registered.URL)
	if err != nil {
		fatalError(command, jsonOutput, err)
	}
	if command == "demo" {
		viewerURL, err = markDemoURL(viewerURL)
		if err != nil {
			fatalError(command, jsonOutput, err)
		}
	}
	output := openResult{
		URL: viewerURL, Path: registered.Path, SourceID: registered.SourceID,
		Command: command, Mode: "daemon", Started: ensured.Started,
	}
	human := fmt.Sprintf("Opened synthetic RLViz demo at %s", output.URL)
	if command == "open" {
		human = fmt.Sprintf("Opened %s at %s", output.Path, output.URL)
	}
	writeOutput(output, jsonOutput, human)
	if !noOpen {
		if err := openBrowser(viewerURL); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
}

func markDemoURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse demo viewer URL: %w", err)
	}
	query := parsed.Query()
	query.Set("demo", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
