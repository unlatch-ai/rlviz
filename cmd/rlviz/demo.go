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

	gallerydata "github.com/TheSnakeFang/rlviz/examples/gallery"
	fixturedata "github.com/TheSnakeFang/rlviz/fixtures"
	"github.com/TheSnakeFang/rlviz/internal/daemon"
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
	galleryPaths, err := ensureGallerySources(paths)
	if err != nil {
		fatalError("demo", *jsonOutput, err)
	}
	openGallery(galleryPaths, *noOpen, *jsonOutput)
}

func ensureGallerySources(paths daemon.Paths) ([]string, error) {
	directory := filepath.Join(paths.RuntimeDir, "gallery")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create gallery directory: %w", err)
	}
	result := make([]string, 0, len(gallerydata.Names))
	for _, name := range gallerydata.Names {
		content, err := gallerydata.Files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read embedded gallery %s: %w", name, err)
		}
		path := filepath.Join(directory, name)
		current, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(current, content) {
			if err := os.Chmod(path, 0o600); err != nil {
				return nil, err
			}
			result = append(result, path)
			continue
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read gallery source %s: %w", name, readErr)
		}
		temporary, err := os.CreateTemp(directory, ".gallery-*.ndjson")
		if err != nil {
			return nil, err
		}
		temporaryName := temporary.Name()
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryName)
			return nil, err
		}
		if _, err := temporary.Write(content); err != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryName)
			return nil, err
		}
		if err := temporary.Close(); err != nil {
			_ = os.Remove(temporaryName)
			return nil, err
		}
		if err := os.Rename(temporaryName, path); err != nil {
			_ = os.Remove(temporaryName)
			return nil, err
		}
		result = append(result, path)
	}
	return result, nil
}

func openGallery(paths []string, noOpen, jsonOutput bool) {
	daemonPaths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("demo", jsonOutput, err)
	}
	executable, err := os.Executable()
	if err != nil {
		fatalError("demo", jsonOutput, fmt.Errorf("locate rlviz executable: %w", err))
	}
	manager := daemon.Manager{Paths: daemonPaths, Executable: executable, Args: []string{"daemon", "serve", "--runtime-dir", daemonPaths.RuntimeDir}, Version: version}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ensured, err := manager.Ensure(ctx)
	if err != nil {
		fatalError("demo", jsonOutput, err)
	}
	registeredPaths := make([]string, 0, len(paths))
	sourceIDs := make([]string, 0, len(paths))
	viewerURL := ""
	for _, path := range paths {
		registered, registerErr := (daemon.Client{}).Register(ctx, ensured.Metadata, daemon.RegisterRequest{Path: path})
		if registerErr != nil {
			fatalError("demo", jsonOutput, registerErr)
		}
		registeredPaths = append(registeredPaths, registered.Path)
		sourceIDs = append(sourceIDs, registered.SourceID)
		viewerURL, err = resolveViewerURL(ensured.Metadata, registered.URL)
		if err != nil {
			fatalError("demo", jsonOutput, err)
		}
	}
	viewerURL, err = markDemoURL(viewerURL)
	if err != nil {
		fatalError("demo", jsonOutput, err)
	}
	output := openResult{URL: viewerURL, Path: registeredPaths[len(registeredPaths)-1], SourceID: sourceIDs[len(sourceIDs)-1], Paths: registeredPaths, SourceIDs: sourceIDs, Command: "demo", Mode: "daemon", Started: ensured.Started}
	writeOutput(output, jsonOutput, fmt.Sprintf("Opened 3-source synthetic RLViz gallery at %s", viewerURL))
	if !noOpen {
		if err := openBrowser(viewerURL); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
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
