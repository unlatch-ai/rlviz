package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheSnakeFang/rlviz/integrations"
)

const agentSetupSchemaVersion = "1"

type agentSetupResult struct {
	SchemaVersion        string `json:"schema_version"`
	Command              string `json:"command"`
	Mode                 string `json:"mode"`
	Status               string `json:"status"`
	Agent                string `json:"agent"`
	Source               string `json:"source"`
	SuggestedDestination string `json:"suggested_destination"`
	Destination          string `json:"destination,omitempty"`
	WritePolicy          string `json:"write_policy"`
	ContentSHA256        string `json:"content_sha256"`
	Content              string `json:"content"`
}

type agentSetupOptions struct {
	Agent       string
	Mode        string
	Destination string
	JSON        bool
}

func runSetup(arguments []string) {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "-h" || arguments[0] == "--help" {
		printSetupHelp()
		return
	}
	if arguments[0] != "agent" {
		err := fmt.Errorf("unknown setup command %q", arguments[0])
		if setupJSONRequested(arguments) {
			writeError("setup_agent", true, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		printSetupHelpTo(os.Stderr)
		os.Exit(2)
	}
	runSetupAgent(arguments[1:])
}

func runSetupAgent(arguments []string) {
	options, err := parseAgentSetupOptions(arguments)
	if err != nil {
		if options.JSON {
			writeError("setup_agent", true, err)
		} else {
			fmt.Fprintln(os.Stderr, err)
			printSetupAgentUsage(os.Stderr)
		}
		os.Exit(2)
	}

	result, err := executeAgentSetup(options)
	if err != nil {
		fatalError("setup_agent", options.JSON, err)
	}
	if options.JSON {
		writeOutput(result, true, "")
		return
	}
	switch result.Mode {
	case "print":
		fmt.Print(result.Content)
	case "dry_run":
		fmt.Printf("Would create %s (create-only; existing files are refused).\n\n%s", result.Destination, result.Content)
	case "write":
		fmt.Printf("Created %s\n", result.Destination)
	}
}

func executeAgentSetup(options agentSetupOptions) (agentSetupResult, error) {
	result, err := prepareAgentSetup(options.Agent, options.Mode, options.Destination)
	if err != nil {
		return agentSetupResult{}, err
	}
	if options.Mode == "write" {
		if err := createAgentSetupFile(result.Destination, result.Content); err != nil {
			return agentSetupResult{}, err
		}
		result.Status = "created"
	}
	return result, nil
}

func parseAgentSetupOptions(arguments []string) (agentSetupOptions, error) {
	options := agentSetupOptions{JSON: setupJSONRequested(arguments)}
	flags := flag.NewFlagSet("setup agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	printOutput := flags.Bool("print", false, "print bundled instructions without writing files")
	dryRun := flags.Bool("dry-run", false, "validate a create-only write without changing files")
	write := flags.Bool("write", false, "create the destination file; never replace an existing file")
	destination := flags.String("destination", "", "project-relative destination for --dry-run or --write")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	if err := flags.Parse(normalizeSetupAgentArguments(arguments)); err != nil {
		return options, err
	}
	options.JSON = *jsonOutput
	modeCount := boolCount(*printOutput, *dryRun, *write)
	if flags.NArg() != 1 || modeCount != 1 || (*printOutput && *destination != "") || (!*printOutput && *destination == "") {
		return options, errors.New("choose exactly one setup mode; --dry-run and --write require --destination, while --print does not accept it")
	}
	options.Agent = flags.Arg(0)
	options.Mode = setupMode(*printOutput, *dryRun)
	options.Destination = *destination
	return options, nil
}

func setupJSONRequested(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--json" || argument == "-json" {
			return true
		}
		for _, prefix := range []string{"--json=", "-json="} {
			if strings.HasPrefix(argument, prefix) {
				value := strings.TrimPrefix(argument, prefix)
				return value != "false" && value != "0"
			}
		}
	}
	return false
}

func printSetupAgentUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: rlviz setup agent <codex|claude-code|cursor> (--print | --dry-run --destination PATH | --write --destination PATH) [--json]")
}

func loadAgentSetup(name string) (agentSetupResult, error) {
	return prepareAgentSetup(name, "print", "")
}

func prepareAgentSetup(name, mode, destination string) (agentSetupResult, error) {
	setup, err := integrations.Agent(name)
	if err != nil {
		return agentSetupResult{}, err
	}
	if mode != "print" && mode != "dry_run" && mode != "write" {
		return agentSetupResult{}, fmt.Errorf("unsupported setup mode %q", mode)
	}
	cleanDestination := ""
	status := "ready"
	policy := "read_only"
	if mode != "print" {
		cleanDestination, err = validateAgentSetupDestination(destination)
		if err != nil {
			return agentSetupResult{}, err
		}
		status = "would_create"
		policy = "create_only"
	}
	hash := sha256.Sum256([]byte(setup.Content))
	return agentSetupResult{
		SchemaVersion:        agentSetupSchemaVersion,
		Command:              "setup_agent",
		Mode:                 mode,
		Status:               status,
		Agent:                setup.Agent,
		Source:               setup.Source,
		SuggestedDestination: setup.SuggestedDestination,
		Destination:          cleanDestination,
		WritePolicy:          policy,
		ContentSHA256:        fmt.Sprintf("%x", hash),
		Content:              setup.Content,
	}, nil
}

func validateAgentSetupDestination(destination string) (string, error) {
	if destination == "" || filepath.IsAbs(destination) {
		return "", fmt.Errorf("destination must be a non-empty path relative to the current project")
	}
	clean := filepath.Clean(destination)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("destination must stay within the current project")
	}

	root, err := os.OpenRoot(".")
	if err != nil {
		return "", fmt.Errorf("open current project: %w", err)
	}
	defer root.Close()
	if err := rejectAgentSetupSymlinks(root, clean, true); err != nil {
		return "", err
	}
	if _, err := root.Lstat(clean); err == nil {
		return "", fmt.Errorf("destination %q already exists; setup is create-only", filepath.ToSlash(clean))
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect destination %q: %w", filepath.ToSlash(clean), err)
	}
	return filepath.ToSlash(clean), nil
}

func createAgentSetupFile(destination, content string) error {
	cleanDestination, err := validateAgentSetupDestination(destination)
	if err != nil {
		return err
	}
	destination = cleanDestination

	root, err := os.OpenRoot(".")
	if err != nil {
		return fmt.Errorf("open current project: %w", err)
	}
	defer root.Close()

	nativeDestination := filepath.FromSlash(destination)
	parent := filepath.Dir(nativeDestination)
	if err := createAgentSetupParents(root, parent); err != nil {
		return err
	}
	if err := rejectAgentSetupSymlinks(root, nativeDestination, true); err != nil {
		return err
	}
	file, err := root.OpenFile(nativeDestination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("destination %q already exists; setup is create-only", destination)
		}
		return fmt.Errorf("create destination %q: %w", destination, err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = root.Remove(nativeDestination)
		return fmt.Errorf("write destination %q: %w", destination, err)
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(nativeDestination)
		return fmt.Errorf("close destination %q: %w", destination, err)
	}
	return nil
}

func createAgentSetupParents(root *os.Root, parent string) error {
	if parent == "." {
		return nil
	}
	parts := strings.Split(filepath.Clean(parent), string(filepath.Separator))
	for index := range parts {
		component := filepath.Join(parts[:index+1]...)
		info, err := root.Lstat(component)
		if os.IsNotExist(err) {
			if err := root.Mkdir(component, 0o755); err != nil {
				return fmt.Errorf("create destination directory %q: %w", filepath.ToSlash(component), err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect destination component %q: %w", filepath.ToSlash(component), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination component %q is a symbolic link", filepath.ToSlash(component))
		}
		if !info.IsDir() {
			return fmt.Errorf("destination component %q is not a directory", filepath.ToSlash(component))
		}
	}
	return nil
}

func rejectAgentSetupSymlinks(root *os.Root, destination string, includeLeaf bool) error {
	parts := strings.Split(filepath.Clean(destination), string(filepath.Separator))
	if !includeLeaf {
		parts = parts[:len(parts)-1]
	}
	for index := range parts {
		component := filepath.Join(parts[:index+1]...)
		info, err := root.Lstat(component)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect destination component %q: %w", filepath.ToSlash(component), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination component %q is a symbolic link", filepath.ToSlash(component))
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("destination component %q is not a directory", filepath.ToSlash(component))
		}
	}
	return nil
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func setupMode(printOutput, dryRun bool) string {
	if printOutput {
		return "print"
	}
	if dryRun {
		return "dry_run"
	}
	return "write"
}

func normalizeSetupAgentArguments(arguments []string) []string {
	flags := make([]string, 0, len(arguments))
	positional := make([]string, 0, 1)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--destination" {
			flags = append(flags, argument)
			if index+1 < len(arguments) {
				index++
				flags = append(flags, arguments[index])
			}
		} else if strings.HasPrefix(argument, "-") {
			flags = append(flags, argument)
		} else {
			positional = append(positional, argument)
		}
	}
	return append(flags, positional...)
}

func printSetupHelp() {
	printSetupHelpTo(os.Stdout)
}

func printSetupHelpTo(output *os.File) {
	fmt.Fprint(output, `RLViz setup

Inspect or create version-matched coding-agent instructions. Writes are explicit and create-only.

Usage:
  rlviz setup agent <codex|claude-code|cursor> --print [--json]
  rlviz setup agent <codex|claude-code|cursor> --dry-run --destination PATH [--json]
  rlviz setup agent <codex|claude-code|cursor> --write --destination PATH [--json]
`)
}
