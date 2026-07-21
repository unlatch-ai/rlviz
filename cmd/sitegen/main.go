package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type page struct {
	Title       string
	Description string
	Source      string
	Output      string
}

var pages = []page{
	{Title: "Overview", Description: "Install RLViz and open a rollout in the local viewer.", Source: "site/index.md", Output: "index.html"},
	{Title: "README", Description: "Repository overview, commands, installation options, and contributor entry points.", Source: "README.md", Output: "readme.html"},
	{Title: "Product spec", Description: "Product scope, users, core concepts, and expected behavior.", Source: "docs/product-spec.md", Output: "product-spec.html"},
	{Title: "Interaction spec", Description: "Keyboard-first interaction model and visual semantics for the browser and TUI.", Source: "docs/interaction-spec.md", Output: "interaction-spec.html"},
	{Title: "Onboarding", Description: "Setup, diagnostics, format inspection, and first-run workflows.", Source: "docs/onboarding.md", Output: "onboarding.html"},
	{Title: "Adapter authoring", Description: "Safe workflow for building, trusting, validating, and using source adapters.", Source: "docs/adapter-authoring.md", Output: "adapter-authoring.html"},
}

const siteURL = "https://rlviz.dev"

func main() {
	output := flag.String("output", "site/dist", "site output directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/sitegen [-output site/dist]")
		os.Exit(2)
	}
	if err := build(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(output string) error {
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	contents := make(map[string][]byte, len(pages))
	for _, current := range pages {
		content, err := os.ReadFile(current.Source)
		if err != nil {
			return fmt.Errorf("read %s: %w", current.Source, err)
		}
		contents[current.Output] = content
		document := layout(current, markdown(content))
		if err := os.WriteFile(filepath.Join(output, current.Output), []byte(document), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", current.Output, err)
		}
		rawName := rawOutput(current)
		if err := os.WriteFile(filepath.Join(output, rawName), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rawName, err)
		}
	}
	artifacts := map[string][]byte{
		"CNAME":         []byte("rlviz.dev"),
		"llms.txt":      []byte(llmsManifest()),
		"llms-full.txt": []byte(llmsFull(contents)),
		"style.css":     []byte(styles),
	}
	for name, content := range artifacts {
		if err := os.WriteFile(filepath.Join(output, name), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func rawOutput(current page) string {
	return strings.TrimSuffix(current.Output, filepath.Ext(current.Output)) + ".md"
}

func rawURL(current page) string {
	return siteURL + "/" + rawOutput(current)
}

func llmsManifest() string {
	var output strings.Builder
	output.WriteString("# RLViz documentation\n\n")
	output.WriteString("> Local-first rollout viewer documentation in raw Markdown.\n\n")
	for _, current := range pages {
		fmt.Fprintf(&output, "- [%s](%s): %s\n", current.Title, rawURL(current), current.Description)
	}
	return output.String()
}

func llmsFull(contents map[string][]byte) string {
	var output strings.Builder
	for index, current := range pages {
		if index > 0 {
			output.WriteString("\n")
		}
		fmt.Fprintf(&output, "========================================\nPAGE: %s\nSOURCE: %s\n========================================\n\n", current.Title, rawURL(current))
		content := contents[current.Output]
		output.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func layout(current page, body string) string {
	var navigation strings.Builder
	for _, candidate := range pages {
		class := ""
		if candidate.Output == current.Output {
			class = ` class="active" aria-current="page"`
		}
		fmt.Fprintf(&navigation, `<a href="%s"%s>%s</a>`, candidate.Output, class, html.EscapeString(candidate.Title))
	}
	return "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"color-scheme\" content=\"dark\"><title>" + html.EscapeString(current.Title) + " · RLViz</title><link rel=\"stylesheet\" href=\"style.css\"></head><body><aside><a class=\"brand\" href=\"index.html\"><b>RLViz</b><span>rollout instrument</span></a><nav>" + navigation.String() + "</nav><footer>local-first · source-read-only</footer></aside><main>" + body + "</main></body></html>"
}

func markdown(source []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(source))
	var output, paragraph strings.Builder
	inCode, inList, inQuote := false, false, false
	codeLanguage := ""
	flushParagraph := func() {
		if paragraph.Len() > 0 {
			fmt.Fprintf(&output, "<p>%s</p>", inline(strings.TrimSpace(paragraph.String())))
			paragraph.Reset()
		}
	}
	closeBlocks := func() {
		flushParagraph()
		if inList {
			output.WriteString("</ul>")
			inList = false
		}
		if inQuote {
			output.WriteString("</blockquote>")
			inQuote = false
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			if inCode {
				output.WriteString("</code></pre>")
				inCode = false
			} else {
				closeBlocks()
				codeLanguage = strings.TrimSpace(strings.TrimPrefix(line, "```"))
				fmt.Fprintf(&output, `<pre data-language="%s"><code>`, html.EscapeString(codeLanguage))
				inCode = true
			}
			continue
		}
		if inCode {
			output.WriteString(html.EscapeString(line) + "\n")
			continue
		}
		if strings.TrimSpace(line) == "" {
			closeBlocks()
			continue
		}
		if level := headingLevel(line); level > 0 {
			closeBlocks()
			text := strings.TrimSpace(line[level:])
			fmt.Fprintf(&output, "<h%d id=\"%s\">%s</h%d>", level, slug(text), inline(text), level)
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || orderedItem(line) {
			flushParagraph()
			if !inList {
				output.WriteString("<ul>")
				inList = true
			}
			item := strings.TrimSpace(line[2:])
			if orderedItem(line) {
				item = strings.TrimSpace(line[strings.Index(line, ".")+1:])
			}
			fmt.Fprintf(&output, "<li>%s</li>", inline(item))
			continue
		}
		if strings.HasPrefix(line, "> ") {
			flushParagraph()
			if !inQuote {
				output.WriteString("<blockquote>")
				inQuote = true
			}
			output.WriteString(inline(strings.TrimPrefix(line, "> ")) + " ")
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			closeBlocks()
			fmt.Fprintf(&output, "<pre class=\"table\">%s</pre>", html.EscapeString(line))
			continue
		}
		if paragraph.Len() > 0 {
			paragraph.WriteByte(' ')
		}
		paragraph.WriteString(line)
	}
	closeBlocks()
	if inCode {
		output.WriteString("</code></pre>")
	}
	return output.String()
}

func headingLevel(line string) int {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level > 0 && level < len(line) && line[level] == ' ' {
		return level
	}
	return 0
}

func orderedItem(line string) bool {
	index := strings.Index(line, ". ")
	if index <= 0 || index > 3 {
		return false
	}
	for _, character := range line[:index] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

var linkPattern = regexp.MustCompile(`\[([^]]+)\]\(([^)]+)\)`)
var codePattern = regexp.MustCompile("`([^`]+)`")

func inline(value string) string {
	escaped := html.EscapeString(value)
	escaped = linkPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) != 3 || !allowedHref(html.UnescapeString(parts[2])) {
			return `<a>` + parts[1] + `</a>`
		}
		return `<a href="` + parts[2] + `">` + parts[1] + `</a>`
	})
	escaped = codePattern.ReplaceAllString(escaped, `<code>$1</code>`)
	return escaped
}

func allowedHref(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") || strings.HasPrefix(lower, "mailto:") {
		return true
	}
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	return !regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`).MatchString(trimmed)
}

func slug(value string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
		} else if result.Len() > 0 && !strings.HasSuffix(result.String(), "-") {
			result.WriteByte('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

const styles = `:root{color-scheme:dark;--canvas:#0d0d0d;--panel:#151515;--raised:#1d1d1d;--line:#343434;--text:#ededed;--muted:#9b9b9b;--focus:#f5f5f5}*{box-sizing:border-box}html{font:15px/1.62 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--canvas);color:var(--text)}body{margin:0;display:grid;grid-template-columns:260px minmax(0,780px);gap:clamp(2rem,6vw,7rem);min-height:100vh}aside{position:sticky;top:0;height:100vh;padding:2rem 1.5rem;border-right:1px solid var(--line);background:var(--panel);display:flex;flex-direction:column}.brand{display:flex;flex-direction:column;color:var(--text);text-decoration:none;margin-bottom:2rem}.brand b{font:700 1.35rem/1 sans-serif;letter-spacing:.08em}.brand span,aside footer{color:var(--muted);font-size:.72rem;text-transform:uppercase;letter-spacing:.08em}nav{display:grid;gap:.3rem}nav a{color:var(--muted);text-decoration:none;padding:.55rem .7rem;border-left:2px solid transparent}nav a:hover,nav a.active{color:var(--text);background:var(--raised);border-left-color:var(--focus)}aside footer{margin-top:auto}main{padding:5rem 1rem 8rem;min-width:0}h1,h2,h3{font-family:Inter,ui-sans-serif,system-ui,sans-serif;line-height:1.15;letter-spacing:-.025em}h1{font-size:clamp(2.4rem,7vw,4.5rem);margin:0 0 2rem}h2{font-size:1.6rem;margin-top:3.8rem;padding-top:1rem;border-top:1px solid var(--line)}h3{font-size:1.15rem;margin-top:2.5rem}p,li{max-width:74ch;color:#d2d2d2}a{color:#fff;text-underline-offset:.2em}code{background:var(--raised);border:1px solid var(--line);padding:.1rem .3rem;border-radius:2px}pre{overflow:auto;background:#090909;border:1px solid var(--line);padding:1rem;color:#ddd}pre code{border:0;padding:0;background:none}.table{margin:.15rem 0;padding:.45rem .7rem}blockquote{border-left:2px solid #777;margin:1.5rem 0;padding:.2rem 1rem;color:var(--muted)}@media(max-width:780px){body{display:block}aside{position:relative;height:auto;border-right:0;border-bottom:1px solid var(--line)}nav{grid-template-columns:repeat(2,minmax(0,1fr))}aside footer{display:none}main{padding:3rem 1.25rem 6rem}}`
