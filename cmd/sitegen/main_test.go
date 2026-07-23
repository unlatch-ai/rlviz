package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownRendersCoreStructures(t *testing.T) {
	got := markdown([]byte("# Title\n\nText with `code`.\n\n- **one** starts here\n  and continues here\n- two\n\n```sh\nrlviz demo\n```\n"))
	for _, want := range []string{`<h1 id="title">Title</h1>`, `<code>code</code>`, `<li><strong>one</strong> starts here and continues here</li>`, `rlviz demo`} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "<ul><p>") || strings.Contains(got, "</li><p>") {
		t.Fatalf("list continuation escaped its item: %s", got)
	}
}

func TestMarkdownDropsUnsafeLinkTargets(t *testing.T) {
	got := markdown([]byte("[web](https://rlviz.dev) [mail](mailto:test@example.com) [section](#title) [relative](docs/start.html) [bad](javascript:alert(1)) [data](data:text/html,payload)"))
	for _, want := range []string{`href="https://rlviz.dev"`, `href="mailto:test@example.com"`, `href="#title"`, `href="docs/start.html"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("safe link missing %q: %s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), `href="javascript:`) || strings.Contains(strings.ToLower(got), `href="data:`) {
		t.Fatalf("unsafe href survived: %s", got)
	}
}

func TestBuildWritesEveryPageAndStylesheet(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	output := filepath.Join(t.TempDir(), "dist")
	if err := build(output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"docs.html", "docs.md", "guide.html", "guide.md", "onboarding.html", "onboarding.md", "supported-formats.html", "supported-formats.md",
		"agents.html", "agents.md", "keybindings.html", "keybindings.md", "adapter-authoring.html", "adapter-authoring.md", "data-model.html", "data-model.md", "faq.html", "faq.md",
		"llms.txt", "llms-full.txt", "CNAME", "favicon.svg", "rlviz-social.png", "style.css",
	} {
		content, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || len(content) == 0 {
			t.Fatalf("%s missing or empty: %v", name, err)
		}
	}

	cname, err := os.ReadFile(filepath.Join(output, "CNAME"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cname) != "rlviz.dev" {
		t.Fatalf("CNAME = %q, want exactly %q", cname, "rlviz.dev")
	}

	for _, current := range pages {
		source, err := os.ReadFile(current.Source)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(output, rawOutput(current)))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(source) {
			t.Errorf("%s does not match %s", rawOutput(current), current.Source)
		}
	}
}

func TestLayoutIncludesPublicMetadata(t *testing.T) {
	document := layout(pages[0], "<p>docs</p>")
	for _, want := range []string{
		`<link rel="canonical" href="https://rlviz.dev/docs.html">`,
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">`,
		`<meta property="og:image" content="https://rlviz.dev/rlviz-social.png?v=2">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !strings.Contains(document, want) {
			t.Errorf("layout missing %q", want)
		}
	}
}

func TestLLMSArtifactsCoverEveryPage(t *testing.T) {
	contents := make(map[string][]byte, len(pages))
	for _, current := range pages {
		contents[current.Output] = []byte("content for " + current.Title)
	}

	manifest := llmsManifest()
	full := llmsFull(contents)
	for _, current := range pages {
		for label, value := range map[string]string{
			"manifest title":       current.Title,
			"manifest description": current.Description,
			"manifest raw URL":     rawURL(current),
		} {
			if !strings.Contains(manifest, value) {
				t.Errorf("%s missing %q", label, value)
			}
		}
		for label, value := range map[string]string{
			"full separator":  "PAGE: " + current.Title,
			"full source URL": rawURL(current),
			"full content":    string(contents[current.Output]),
		} {
			if !strings.Contains(full, value) {
				t.Errorf("%s missing %q", label, value)
			}
		}
	}
}
