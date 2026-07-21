package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheSnakeFang/rlviz/integrations"
)

const userConfigSchemaVersion = 1

type userConfig struct {
	SchemaVersion int    `json:"schema_version"`
	OpenMode      string `json:"open_mode"`
}

type initOptions struct {
	Yes         bool
	Interactive bool
	HomeDir     string
	ConfigDir   string
}

type initResult struct {
	OpenGallery bool
}

func runInit(arguments []string) {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	yes := flags.Bool("yes", false, "accept safe defaults without prompting")
	flags.Usage = func() { fmt.Fprintln(flags.Output(), "Usage: rlviz init [--yes]") }
	if err := flags.Parse(arguments); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}
	interactive := stdinIsTerminal(os.Stdin)
	result, err := runInitWizard(os.Stdin, os.Stdout, initOptions{Yes: *yes, Interactive: interactive})
	if err != nil {
		fatalError("init", false, err)
	}
	if result.OpenGallery {
		runDemo(nil)
	}
}

func stdinIsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func runInitWizard(input io.Reader, output io.Writer, options initOptions) (initResult, error) {
	configPath, err := userConfigPath(options.ConfigDir)
	if err != nil {
		return initResult{}, err
	}
	reader := bufio.NewReader(input)
	mode := "browser"
	fmt.Fprintln(output, "RLViz first-run setup")
	if options.Interactive && !options.Yes {
		answer, err := prompt(reader, output, "Default interface: browser, tui, or both? [browser] ")
		if err != nil {
			return initResult{}, err
		}
		if strings.TrimSpace(answer) != "" {
			mode = strings.ToLower(strings.TrimSpace(answer))
		}
		if mode != "browser" && mode != "tui" && mode != "both" {
			return initResult{}, errors.New("interface must be browser, tui, or both")
		}
	}
	if err := writeUserConfig(configPath, userConfig{SchemaVersion: userConfigSchemaVersion, OpenMode: mode}); err != nil {
		return initResult{}, err
	}
	fmt.Fprintf(output, "Saved default interface %q to %s\n", mode, configPath)

	if options.Interactive && !options.Yes {
		selection, err := prompt(reader, output, "Install agent skills? Enter codex, claude-code, cursor (comma-separated), or none [none] ")
		if err != nil {
			return initResult{}, err
		}
		for _, agent := range parseAgentSelection(selection) {
			destination, content, err := initAgentFile(agent, options.HomeDir)
			if err != nil {
				return initResult{}, err
			}
			fmt.Fprintf(output, "\nWill write %s:\n---\n%s---\n", destination, content)
			confirmed, err := promptYesNo(reader, output, "Write this file? [y/N] ", false)
			if err != nil {
				return initResult{}, err
			}
			if !confirmed {
				fmt.Fprintln(output, "Skipped.")
				continue
			}
			if err := createAbsoluteFile(destination, content); err != nil {
				if errors.Is(err, os.ErrExist) {
					fmt.Fprintf(output, "Skipped existing file %s\n", destination)
					continue
				}
				return initResult{}, err
			}
			fmt.Fprintf(output, "Created %s\n", destination)
		}
	} else {
		fmt.Fprintln(output, "Skipped agent-skill installation (run rlviz init interactively to review files).")
	}

	printAgentPrompt(output)
	openGallery := false
	if options.Interactive && !options.Yes {
		openGallery, err = promptYesNo(reader, output, "Open the synthetic example gallery now? [y/N] ", false)
		if err != nil {
			return initResult{}, err
		}
	}
	if !options.Interactive {
		fmt.Fprintln(output, "Non-interactive stdin detected; skipped optional installs and gallery launch.")
	}
	return initResult{OpenGallery: openGallery}, nil
}

func prompt(reader *bufio.Reader, output io.Writer, text string) (string, error) {
	fmt.Fprint(output, text)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptYesNo(reader *bufio.Reader, output io.Writer, text string, defaultValue bool) (bool, error) {
	answer, err := prompt(reader, output, text)
	if err != nil {
		return false, err
	}
	if answer == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.New("answer must be y or n")
	}
}

func parseAgentSelection(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil
	}
	seen := make(map[string]bool)
	result := make([]string, 0, 3)
	for _, part := range strings.Split(value, ",") {
		agent := strings.ToLower(strings.TrimSpace(part))
		if !seen[agent] {
			seen[agent] = true
			result = append(result, agent)
		}
	}
	return result
}

func initAgentFile(agent, homeOverride string) (string, string, error) {
	setup, err := integrations.Agent(agent)
	if err != nil {
		return "", "", err
	}
	home := homeOverride
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("locate home directory: %w", err)
		}
	}
	switch agent {
	case "codex":
		return filepath.Join(home, ".codex", "skills", "rlviz", "SKILL.md"), skillDocument(setup.Content), nil
	case "claude-code":
		return filepath.Join(home, ".claude", "skills", "rlviz", "SKILL.md"), skillDocument(setup.Content), nil
	case "cursor":
		return filepath.Join(".cursor", "rules", "rlviz.mdc"), setup.Content, nil
	default:
		return "", "", fmt.Errorf("unsupported agent %q", agent)
	}
}

func skillDocument(content string) string {
	return "---\nname: rlviz\ndescription: Inspect and adapt local agent rollout and trajectory traces with RLViz.\n---\n\n" + content
}

func createAbsoluteFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create agent skill directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(file, content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func printAgentPrompt(output io.Writer) {
	fmt.Fprintln(output, `
Point your coding agent at your own traces

Paste this prompt:

Use RLViz to inspect my rollout at <SOURCE>. First run rlviz inspect <SOURCE>. If the format is unsupported, run rlviz plugin init --type adapter --lang python --from <SOURCE> .rlviz/plugins/local-adapter, implement the generated adapter against the smallest representative records, and show me every executable file before asking for explicit approval to run rlviz plugin trust. Never modify the trace or trust an adapter without my review.`)
}

func userConfigPath(override string) (string, error) {
	if override == "" {
		override = os.Getenv("RLVIZ_CONFIG_DIR")
	}
	if override == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate user config directory: %w", err)
		}
		override = filepath.Join(root, "rlviz")
	}
	return filepath.Join(override, "config.json"), nil
}

func loadUserConfig() (userConfig, bool, error) {
	path, err := userConfigPath("")
	if err != nil {
		return userConfig{}, false, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return userConfig{}, false, nil
	}
	if err != nil {
		return userConfig{}, false, fmt.Errorf("read RLViz config: %w", err)
	}
	var config userConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return userConfig{}, true, fmt.Errorf("decode RLViz config: %w", err)
	}
	if config.SchemaVersion != userConfigSchemaVersion || (config.OpenMode != "browser" && config.OpenMode != "tui" && config.OpenMode != "both") {
		return userConfig{}, true, errors.New("RLViz config has an unsupported schema or open_mode")
	}
	return config, true, nil
}

func writeUserConfig(path string, config userConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create RLViz config directory: %w", err)
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return fmt.Errorf("create RLViz config: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("install RLViz config: %w", err)
	}
	return nil
}
