package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/unlatch-ai/rlviz/internal/app"
	"github.com/unlatch-ai/rlviz/internal/daemon"
	"github.com/unlatch-ai/rlviz/internal/plugins"
)

var version = "0.0.0-dev"

type result struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type openResult struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	SourceID string `json:"source_id,omitempty"`
	Command  string `json:"command"`
	Mode     string `json:"mode"`
	Started  bool   `json:"daemon_started,omitempty"`
}

func main() {
	command := "help"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "version":
		runVersion(os.Args[2:])
	case "help", "-h", "--help":
		printHelp()
	case "open":
		runOpen(os.Args[2:])
	case "demo":
		runDemo(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "stop":
		runStop(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "formats":
		runFormats(os.Args[2:])
	case "presentation":
		runPresentation(os.Args[2:])
	case "inspect":
		runInspect(os.Args[2:])
	case "setup":
		runSetup(os.Args[2:])
	case "cache":
		runCache(os.Args[2:])
	case "plugin":
		runPlugin(os.Args[2:])
	case "daemon":
		runInternalDaemon(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		os.Exit(2)
	}
}

func runVersion(arguments []string) {
	flags := flag.NewFlagSet("version", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	printVersion(*jsonOutput)
}

func runOpen(arguments []string) {
	flags := flag.NewFlagSet("open", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	noOpen := flags.Bool("no-open", false, "do not open the browser")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	presentationPath := flags.String("presentation", "", "validated declarative presentation JSON")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rlviz open [--no-open] [--json] [--adapter PATH] [--presentation FILE] SOURCE")
	}
	if err := flags.Parse(normalizeViewerArguments(arguments)); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}

	presentationConfig, err := loadPresentationFile(*presentationPath)
	if err != nil {
		fatalError("open", *jsonOutput, err)
	}
	openSource(flags.Arg(0), *adapter, presentationConfig, *noOpen, *jsonOutput, "open")
}

func runServe(arguments []string) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	open := flags.Bool("open", false, "open the browser")
	port := flags.Int("port", 0, "loopback port (0 selects an available port)")
	jsonOutput := flags.Bool("json", false, "print machine-readable startup output")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	presentationPath := flags.String("presentation", "", "validated declarative presentation JSON")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rlviz serve [--open] [--port PORT] [--json] [--adapter PATH] [--presentation FILE] SOURCE")
	}
	if err := flags.Parse(normalizeViewerArguments(arguments)); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}

	presentationConfig, err := loadPresentationFile(*presentationPath)
	if err != nil {
		fatalError("serve", *jsonOutput, err)
	}
	viewer, err := app.StartViewer(app.Viewer{SourcePath: flags.Arg(0), AdapterPath: *adapter, Presentation: presentationConfig, Port: *port})
	if err != nil {
		fatalError("serve", *jsonOutput, err)
	}
	output := openResult{URL: viewer.URL, Path: viewer.SourcePath, Command: "serve", Mode: "foreground"}
	writeOutput(output, *jsonOutput, fmt.Sprintf("RLViz is serving %s at %s (foreground; press Ctrl-C to stop)", output.Path, output.URL))
	if *open {
		if err := openBrowser(viewer.URL); err != nil {
			fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		}
	}
	if err := viewer.Serve(); err != nil {
		fatalError("serve", *jsonOutput, err)
	}
}

func runStatus(arguments []string) {
	flags := flag.NewFlagSet("status", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("status", *jsonOutput, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	metadata, err := daemon.LoadLiveMetadata(ctx, paths, daemon.Client{})
	if err != nil {
		if errors.Is(err, daemon.ErrNoMetadata) || errors.Is(err, daemon.ErrDaemonUnavailable) {
			writeOutput(map[string]any{"status": "stopped"}, *jsonOutput, "RLViz daemon is stopped")
			return
		}
		fatalError("status", *jsonOutput, err)
	}
	writeOutput(map[string]any{"status": "running", "pid": metadata.PID, "address": metadata.Address, "version": metadata.Version}, *jsonOutput, fmt.Sprintf("RLViz daemon is running at http://%s (pid %d)", metadata.Address, metadata.PID))
}

func runStop(arguments []string) {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("stop", *jsonOutput, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	metadata, err := daemon.LoadLiveMetadata(ctx, paths, daemon.Client{})
	if errors.Is(err, daemon.ErrNoMetadata) || errors.Is(err, daemon.ErrDaemonUnavailable) {
		writeOutput(map[string]string{"status": "stopped"}, *jsonOutput, "RLViz daemon is already stopped")
		return
	}
	if err != nil {
		fatalError("stop", *jsonOutput, err)
	}
	if err := (daemon.Client{}).Stop(ctx, metadata); err != nil {
		fatalError("stop", *jsonOutput, err)
	}
	writeOutput(map[string]string{"status": "stopping"}, *jsonOutput, "RLViz daemon is stopping")
}

func runDoctor(arguments []string) {
	flags := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	dependencies, err := defaultDoctorDependencies()
	if err != nil {
		fatalError("doctor", *jsonOutput, err)
	}
	report := collectDoctorReport(context.Background(), dependencies)
	writeOutput(report, *jsonOutput, formatDoctorReport(report))
}

type cacheStatusResult struct {
	Status        string `json:"status"`
	Path          string `json:"path"`
	SizeBytes     int64  `json:"size_bytes"`
	DaemonRunning bool   `json:"daemon_running"`
}

type cacheCleanResult struct {
	Status  string   `json:"status"`
	Path    string   `json:"path"`
	Removed []string `json:"removed"`
}

func runCache(arguments []string) {
	if len(arguments) == 0 {
		printCacheHelp()
		return
	}
	switch arguments[0] {
	case "status":
		runCacheStatus(arguments[1:])
	case "clean":
		runCacheClean(arguments[1:])
	case "help", "-h", "--help":
		printCacheHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown cache command %q\n", arguments[0])
		os.Exit(2)
	}
}

func runCacheStatus(arguments []string) {
	flags := flag.NewFlagSet("cache status", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: rlviz cache status [--json]")
		os.Exit(2)
	}
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("cache_status", *jsonOutput, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	status, err := inspectCache(paths, func() (bool, error) {
		return daemonIsRunning(ctx, paths)
	})
	if err != nil {
		fatalError("cache_status", *jsonOutput, err)
	}
	human := fmt.Sprintf("RLViz cache is absent at %s (daemon stopped)", status.Path)
	if status.Status == "present" {
		human = fmt.Sprintf("RLViz cache is present at %s (%d bytes; daemon stopped)", status.Path, status.SizeBytes)
	}
	if status.DaemonRunning {
		human = strings.Replace(human, "daemon stopped", "daemon running", 1)
	}
	writeOutput(status, *jsonOutput, human)
}

func runCacheClean(arguments []string) {
	flags := flag.NewFlagSet("cache clean", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: rlviz cache clean [--json]")
		os.Exit(2)
	}
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("cache_clean", *jsonOutput, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := cleanCache(paths, func() (bool, error) {
		return daemonIsRunning(ctx, paths)
	})
	if err != nil {
		fatalError("cache_clean", *jsonOutput, err)
	}
	human := "RLViz cache is already clean"
	if len(result.Removed) > 0 {
		human = fmt.Sprintf("Removed RLViz cache at %s", result.Path)
	}
	writeOutput(result, *jsonOutput, human)
}

func daemonIsRunning(ctx context.Context, paths daemon.Paths) (bool, error) {
	_, err := daemon.LoadLiveMetadata(ctx, paths, daemon.Client{})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, daemon.ErrNoMetadata) || errors.Is(err, daemon.ErrDaemonUnavailable) {
		return false, nil
	}
	return false, err
}

func inspectCache(paths daemon.Paths, live func() (bool, error)) (cacheStatusResult, error) {
	running, err := live()
	if err != nil {
		return cacheStatusResult{}, fmt.Errorf("check daemon status: %w", err)
	}
	result := cacheStatusResult{Status: "absent", Path: paths.IndexFile, DaemonRunning: running}
	info, err := os.Stat(paths.IndexFile)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return cacheStatusResult{}, fmt.Errorf("inspect cache index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return cacheStatusResult{}, fmt.Errorf("cache index is not a regular file: %s", paths.IndexFile)
	}
	result.Status = "present"
	result.SizeBytes = info.Size()
	return result, nil
}

func cleanCache(paths daemon.Paths, live func() (bool, error)) (cacheCleanResult, error) {
	running, err := live()
	if err != nil {
		return cacheCleanResult{}, fmt.Errorf("check daemon status: %w", err)
	}
	if running {
		return cacheCleanResult{}, fmt.Errorf("daemon is running; run `rlviz stop` before cleaning the cache")
	}
	result := cacheCleanResult{Status: "cleaned", Path: paths.IndexFile, Removed: []string{}}
	candidates := []string{paths.IndexFile, paths.IndexFile + "-wal", paths.IndexFile + "-shm"}
	for _, path := range candidates {
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return cacheCleanResult{}, fmt.Errorf("inspect cache file %s: %w", path, statErr)
		}
		if info.IsDir() {
			return cacheCleanResult{}, fmt.Errorf("refusing to remove cache path because it is a directory: %s", path)
		}
	}
	for _, path := range candidates {
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return cacheCleanResult{}, fmt.Errorf("remove cache file %s: %w", path, err)
		}
		result.Removed = append(result.Removed, path)
	}
	return result, nil
}

func runInternalDaemon(arguments []string) {
	if len(arguments) == 0 || arguments[0] != "serve" {
		fmt.Fprintln(os.Stderr, "internal daemon command requires serve")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("daemon serve", flag.ExitOnError)
	runtimeDir := flags.String("runtime-dir", "", "daemon runtime directory")
	_ = flags.Parse(arguments[1:])
	paths, err := daemon.DefaultPaths()
	if err != nil {
		fatalError("daemon", false, err)
	}
	if *runtimeDir != "" {
		paths = daemon.PathsAt(*runtimeDir)
	}
	if err := app.RunDaemon(paths, version); err != nil {
		fatalError("daemon", false, err)
	}
}

func runPlugin(arguments []string) {
	if len(arguments) == 0 {
		printPluginHelp()
		return
	}
	switch arguments[0] {
	case "init":
		runPluginInit(arguments[1:])
	case "trust":
		runPluginTrust(arguments[1:])
	case "validate":
		runPluginValidate(arguments[1:])
	case "list":
		runPluginList(arguments[1:])
	case "revoke":
		runPluginRevoke(arguments[1:])
	case "help", "-h", "--help":
		printPluginHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown plugin command %q\n", arguments[0])
		os.Exit(2)
	}
}

func runPluginInit(arguments []string) {
	flags := flag.NewFlagSet("plugin init", flag.ExitOnError)
	kind := flags.String("type", "adapter", "plugin type")
	language := flags.String("lang", "python", "plugin language")
	name := flags.String("name", "", "plugin name")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 1 || (*kind != "adapter" && *kind != "analyzer") || *language != "python" {
		fmt.Fprintln(os.Stderr, "Usage: rlviz plugin init --type adapter|analyzer --lang python [--name NAME] DIR")
		os.Exit(2)
	}
	destination := flags.Arg(0)
	pluginName := *name
	if pluginName == "" {
		pluginName = safePluginName(filepath.Base(filepath.Clean(destination)))
	}
	if err := plugins.ScaffoldPython(destination, plugins.ScaffoldOptions{Name: pluginName, Kind: *kind}); err != nil {
		fatalError("plugin_init", *jsonOutput, err)
	}
	absolute, _ := filepath.Abs(destination)
	writeOutput(map[string]any{"status": "created", "path": absolute, "name": pluginName, "type": *kind}, *jsonOutput, fmt.Sprintf("Created Python %s %s at %s", *kind, pluginName, absolute))
}

func runPluginTrust(arguments []string) {
	flags := flag.NewFlagSet("plugin trust", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: rlviz plugin trust [--json] DIR")
		os.Exit(2)
	}
	plugin, err := plugins.Load(flags.Arg(0))
	if err != nil {
		fatalError("plugin_trust", *jsonOutput, err)
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("plugin_trust", *jsonOutput, err)
	}
	if err := store.Trust(plugin); err != nil {
		fatalError("plugin_trust", *jsonOutput, err)
	}
	writeOutput(map[string]any{"status": "trusted", "path": plugin.Path, "digest": plugin.Digest}, *jsonOutput, fmt.Sprintf("Trusted %s at digest %s", plugin.Path, plugin.Digest))
}

func runPluginValidate(arguments []string) {
	flags := flag.NewFlagSet("plugin validate", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "Usage: rlviz plugin validate [--json] DIR SOURCE_OR_ANALYZER_INPUT")
		os.Exit(2)
	}
	plugin, err := plugins.Load(flags.Arg(0))
	if err != nil {
		fatalError("plugin_validate", *jsonOutput, err)
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("plugin_validate", *jsonOutput, err)
	}
	host := plugins.NewHost(store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if plugin.Manifest.Kind == "Analyzer" {
		input, err := plugins.LoadAnalyzerInput(flags.Arg(1))
		if err != nil {
			fatalError("plugin_validate", *jsonOutput, err)
		}
		report, err := host.ValidateAnalyzer(ctx, plugin, input)
		if err != nil {
			fatalError("plugin_validate", *jsonOutput, err)
		}
		writeOutput(report, *jsonOutput, fmt.Sprintf("Validated %s: %d findings and %d signals (deterministic)", report.Plugin, report.Findings, report.Signals))
		return
	}
	report, err := host.ValidateAdapter(ctx, plugin, flags.Arg(1), "")
	if err != nil {
		fatalError("plugin_validate", *jsonOutput, err)
	}
	writeOutput(report, *jsonOutput, fmt.Sprintf("Validated %s: %d deterministic records (%s)", report.Plugin, report.Records, report.Format))
}

func runPluginList(arguments []string) {
	flags := flag.NewFlagSet("plugin list", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("plugin_list", *jsonOutput, err)
	}
	entries, err := store.List()
	if err != nil {
		fatalError("plugin_list", *jsonOutput, err)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%s  %s", entry.Digest, entry.Path))
	}
	human := "No trusted RLViz plugins"
	if len(lines) > 0 {
		human = strings.Join(lines, "\n")
	}
	writeOutput(map[string]any{"plugins": entries}, *jsonOutput, human)
}

func runPluginRevoke(arguments []string) {
	flags := flag.NewFlagSet("plugin revoke", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	_ = flags.Parse(arguments)
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: rlviz plugin revoke [--json] DIR")
		os.Exit(2)
	}
	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("plugin_revoke", *jsonOutput, err)
	}
	if err := store.Revoke(flags.Arg(0)); err != nil {
		fatalError("plugin_revoke", *jsonOutput, err)
	}
	absolute, _ := filepath.Abs(flags.Arg(0))
	writeOutput(map[string]any{"status": "revoked", "path": absolute}, *jsonOutput, fmt.Sprintf("Revoked trust for %s", absolute))
}

func resolveViewerURL(metadata daemon.Metadata, value string) (string, error) {
	base, _ := url.Parse("http://" + metadata.Address + "/")
	reference, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse viewer URL: %w", err)
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" || resolved.Host != metadata.Address {
		return "", fmt.Errorf("daemon returned viewer URL outside its loopback origin")
	}
	resolved.Fragment = url.Values{"token": []string{metadata.Token}}.Encode()
	return resolved.String(), nil
}

func normalizeViewerArguments(arguments []string) []string {
	valueFlags := map[string]bool{"--port": true, "--adapter": true, "--presentation": true}
	booleanFlags := map[string]bool{"--no-open": true, "--open": true, "--json": true}
	flags := make([]string, 0, len(arguments))
	paths := make([]string, 0, 1)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if valueFlags[argument] {
			flags = append(flags, argument)
			if index+1 < len(arguments) {
				index++
				flags = append(flags, arguments[index])
			}
		} else if booleanFlags[argument] || strings.HasPrefix(argument, "--port=") || strings.HasPrefix(argument, "--adapter=") || strings.HasPrefix(argument, "--presentation=") {
			flags = append(flags, argument)
		} else if strings.HasPrefix(argument, "--") {
			flags = append(flags, argument)
		} else {
			paths = append(paths, argument)
		}
	}
	return append(flags, paths...)
}

func safePluginName(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "local-adapter"
	}
	return value
}

func writeOutput(value any, jsonOutput bool, human string) {
	if jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
			fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(human)
}

func fatalError(command string, jsonOutput bool, err error) {
	writeError(command, jsonOutput, err)
	os.Exit(1)
}

func writeError(command string, jsonOutput bool, err error) {
	code := command + "_failed"
	details := map[string]any{"code": code, "error": err.Error()}
	var apiError *daemon.APIError
	var unsupported *app.UnsupportedFormatError
	if errors.As(err, &apiError) {
		for key, value := range apiError.Details {
			details[key] = value
		}
		if apiError.Code != "" {
			details["code"] = apiError.Code
		}
		details["error"] = apiError.Message
	} else if errors.As(err, &unsupported) {
		details["code"] = "unsupported_format"
		details["path"] = unsupported.Path
		details["suggested_command"] = "rlviz plugin init --type adapter --lang python .rlviz/plugins/local-adapter"
	} else if errors.Is(err, plugins.ErrUntrusted) {
		details["code"] = "plugin_untrusted"
	}
	if jsonOutput {
		_ = json.NewEncoder(os.Stderr).Encode(details)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", command, err)
}

func openBrowser(value string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", value)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", value)
	default:
		command = exec.Command("xdg-open", value)
	}
	return command.Start()
}

func printVersion(jsonOutput bool) {
	value := result{Name: "rlviz", Version: version}
	writeOutput(value, jsonOutput, fmt.Sprintf("rlviz %s", value.Version))
}

func printHelp() {
	fmt.Print(`RLViz

Visualize and compare agent rollouts.

Usage:
  rlviz demo [--no-open] [--json]
  rlviz open [--no-open] [--json] [--adapter PATH] [--presentation FILE] SOURCE
  rlviz serve [--open] [--port PORT] [--json] [--adapter PATH] [--presentation FILE] SOURCE
  rlviz status [--json]
  rlviz stop [--json]
  rlviz doctor [--json]
  rlviz formats [--json] [--project DIR] [--plugin-root DIR]...
  rlviz presentation validate [--json] FILE
  rlviz inspect [--json] [--adapter PATH] SOURCE
  rlviz setup agent <codex|claude-code|cursor> (--print | --dry-run --destination PATH | --write --destination PATH) [--json]
  rlviz cache <status|clean>
  rlviz plugin <init|trust|validate|list|revoke>
  rlviz version [--json]
  rlviz help
`)
}

func printCacheHelp() {
	fmt.Print(`RLViz cache

Usage:
  rlviz cache status [--json]
  rlviz cache clean [--json]
`)
}

func printPluginHelp() {
	fmt.Print(`RLViz plugins

Usage:
  rlviz plugin init --type adapter|analyzer --lang python [--name NAME] DIR
  rlviz plugin trust [--json] DIR
  rlviz plugin validate [--json] DIR SOURCE_OR_ANALYZER_INPUT
  rlviz plugin list [--json]
  rlviz plugin revoke [--json] DIR
`)
}
