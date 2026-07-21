package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheSnakeFang/rlviz/internal/presentation"
)

type presentationValidationResult struct {
	SchemaVersion int                 `json:"schema_version"`
	Status        string              `json:"status"`
	Path          string              `json:"path"`
	Digest        string              `json:"digest"`
	Config        presentation.Config `json:"config"`
	Normalized    json.RawMessage     `json:"-"`
}

func runPresentation(arguments []string) {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "-h" || arguments[0] == "--help" {
		printPresentationHelp()
		return
	}
	if arguments[0] != "validate" {
		fmt.Fprintf(os.Stderr, "unknown presentation command %q\n", arguments[0])
		os.Exit(2)
	}
	runPresentationValidate(arguments[1:])
}

func runPresentationValidate(arguments []string) {
	flags := flag.NewFlagSet("presentation validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	flags.Usage = func() { fmt.Fprintln(flags.Output(), "Usage: rlviz presentation validate [--json] FILE") }
	if err := flags.Parse(arguments); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}

	result, err := validatePresentationFile(flags.Arg(0))
	if err != nil {
		fatalError("presentation_validate", *jsonOutput, err)
	}
	human := fmt.Sprintf("Valid RLViz presentation configuration: %s (%d fields, %d scalar formats, %d columns, %d theme overrides)", result.Path, len(result.Config.Fields), len(result.Config.Scalars), len(result.Config.Group.Columns), len(result.Config.Theme))
	writeOutput(result, *jsonOutput, human)
}

func validatePresentationFile(path string) (presentationValidationResult, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return presentationValidationResult{}, fmt.Errorf("resolve presentation path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return presentationValidationResult{}, fmt.Errorf("resolve presentation path: %w", err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return presentationValidationResult{}, fmt.Errorf("open presentation configuration: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return presentationValidationResult{}, fmt.Errorf("inspect presentation configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return presentationValidationResult{}, fmt.Errorf("presentation configuration must be a regular file")
	}
	config, err := presentation.Load(file)
	if err != nil {
		return presentationValidationResult{}, err
	}
	normalized, err := presentation.Normalize(config)
	if err != nil {
		return presentationValidationResult{}, fmt.Errorf("normalize presentation configuration: %w", err)
	}
	digest := sha256.Sum256(normalized)
	return presentationValidationResult{SchemaVersion: 1, Status: "valid", Path: resolved, Digest: fmt.Sprintf("sha256:%x", digest), Config: config, Normalized: normalized}, nil
}

func loadPresentationFile(path string) (json.RawMessage, error) {
	if path == "" {
		return nil, nil
	}
	result, err := validatePresentationFile(path)
	if err != nil {
		return nil, err
	}
	return result.Normalized, nil
}

func printPresentationHelp() {
	fmt.Print(`RLViz presentation configuration

Usage:
  rlviz presentation validate [--json] FILE
`)
}
