package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWizardInteractiveWritesPreferenceAndConfirmedSkill(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	home := filepath.Join(root, "home")
	configDir := filepath.Join(root, "config")
	input := strings.NewReader("tui\ncodex,cursor\ny\nn\nn\n")
	var output bytes.Buffer
	result, err := runInitWizard(input, &output, initOptions{Interactive: true, HomeDir: home, ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.OpenGallery {
		t.Fatal("gallery opened after no confirmation")
	}
	config, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil || !strings.Contains(string(config), `"open_mode": "tui"`) {
		t.Fatalf("config = %q, %v", config, err)
	}
	skillPath := filepath.Join(home, ".codex", "skills", "rlviz", "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil || !strings.Contains(string(skill), "# RLViz trace workflow") || !strings.HasPrefix(string(skill), "---\nname: rlviz") {
		t.Fatalf("skill = %q, %v", skill, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "rlviz.mdc")); !os.IsNotExist(err) {
		t.Fatalf("cursor file should be skipped: %v", err)
	}
	if !strings.Contains(output.String(), skillPath) || !strings.Contains(output.String(), "rlviz inspect <SOURCE>") || !strings.Contains(output.String(), "rlviz plugin trust") {
		t.Fatalf("wizard output missing preview or prompt:\n%s", output.String())
	}
}

func TestInitWizardYesAndNonTTYNeverReadInput(t *testing.T) {
	for _, test := range []struct {
		name    string
		options initOptions
	}{
		{name: "yes", options: initOptions{Yes: true, Interactive: true}},
		{name: "non-tty", options: initOptions{Interactive: false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.options.ConfigDir = root
			var output bytes.Buffer
			result, err := runInitWizard(failingReader{}, &output, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if result.OpenGallery {
				t.Fatal("unattended setup opened gallery")
			}
			content, err := os.ReadFile(filepath.Join(root, "config.json"))
			if err != nil || !strings.Contains(string(content), `"open_mode": "browser"`) {
				t.Fatalf("default config = %q, %v", content, err)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, os.ErrPermission }

func TestLoadUserConfigUsesOverride(t *testing.T) {
	t.Setenv("RLVIZ_CONFIG_DIR", t.TempDir())
	path, err := userConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeUserConfig(path, userConfig{SchemaVersion: 1, OpenMode: "both"}); err != nil {
		t.Fatal(err)
	}
	config, exists, err := loadUserConfig()
	if err != nil || !exists || config.OpenMode != "both" {
		t.Fatalf("config = %#v, exists=%v, err=%v", config, exists, err)
	}
}

func TestPreferredOpenModeHonorsConfigAndExplicitAutomation(t *testing.T) {
	config := userConfig{SchemaVersion: 1, OpenMode: "both"}
	if got := preferredOpenMode(config, true, false, false); got != "both" {
		t.Fatalf("configured mode = %q", got)
	}
	if got := preferredOpenMode(config, true, true, false); got != "tui" {
		t.Fatalf("explicit TUI mode = %q", got)
	}
	if got := preferredOpenMode(config, true, false, true); got != "browser" {
		t.Fatalf("JSON automation mode = %q", got)
	}
	if got := preferredOpenMode(userConfig{}, false, false, false); got != "browser" {
		t.Fatalf("unconfigured mode = %q", got)
	}
}
