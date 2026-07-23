import { useState } from "react";
import overviewMarkdown from "../../internal/guide/pages/overview.md?raw";
import installMarkdown from "../../internal/guide/pages/install.md?raw";
import workspaceMarkdown from "../../internal/guide/pages/workspace.md?raw";
import formatsMarkdown from "../../internal/guide/pages/formats.md?raw";
import agentsMarkdown from "../../internal/guide/pages/agents.md?raw";
import privacyMarkdown from "../../internal/guide/pages/privacy.md?raw";
import shortcutsMarkdown from "../../internal/guide/pages/shortcuts.md?raw";
import patchNotesMarkdown from "../../internal/guide/pages/patch-notes.md?raw";
import type { ViewerSetup } from "./Settings";

type Block = { kind: "h1" | "h2" | "p" | "li" | "code"; text: string };
const pages = [
  { id: "overview", label: "Overview", markdown: overviewMarkdown },
  { id: "install", label: "Install", markdown: installMarkdown },
  { id: "workspace", label: "Workspace", markdown: workspaceMarkdown },
  { id: "shortcuts", label: "Keybindings", markdown: shortcutsMarkdown },
  { id: "formats", label: "Formats", markdown: formatsMarkdown },
  { id: "agents", label: "Agents", markdown: agentsMarkdown },
  { id: "privacy", label: "Privacy", markdown: privacyMarkdown },
  { id: "patch-notes", label: "Patch notes", markdown: patchNotesMarkdown },
] as const;

function blocks(markdown: string): Block[] {
  const result: Block[] = [];
  let code: string[] | undefined;
  markdown.split("\n").forEach((line) => {
    const text = line.trim();
    if (text.startsWith("```")) {
      if (code) { result.push({ kind: "code", text: code.join("\n") }); code = undefined; }
      else code = [];
      return;
    }
    if (code) { code.push(line); return; }
    if (!text) return;
    if (text.startsWith("# ")) result.push({ kind: "h1", text: text.slice(2) });
    else if (text.startsWith("## ")) result.push({ kind: "h2", text: text.slice(3) });
    else if (text.startsWith("- ")) result.push({ kind: "li", text: text.slice(2) });
    else result.push({ kind: "p", text });
  });
  return result;
}

function Inline({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*|\[[^\]]+\]\(https:\/\/[^)]+\))/g);
  return <>{parts.map((part, index) => {
    const link = part.match(/^\[([^\]]+)\]\((https:\/\/[^)]+)\)$/);
    if (link) return <a key={index} href={link[2]} target="_blank" rel="noreferrer">{link[1]}</a>;
    if (part.startsWith("`")) return <code key={index}>{part.slice(1, -1)}</code>;
    if (part.startsWith("**")) return <strong key={index}>{part.slice(2, -2)}</strong>;
    return part;
  })}</>;
}

function GuideLink({ href, label }: { href: string; label: string }) {
  return <a className="guide-link" href={href} target="_blank" rel="noreferrer">
    {href === "https://rlviz.dev" ? <img src="/favicon.svg" alt="" /> : href.includes("github.com") ? <svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M8 0a8 8 0 0 0-2.53 15.59c.4.07.55-.17.55-.39l-.01-1.49c-2.23.49-2.7-1.08-2.7-1.08-.37-.93-.89-1.18-.89-1.18-.73-.5.05-.49.05-.49.81.06 1.23.83 1.23.83.72 1.23 1.88.88 2.34.67.07-.52.28-.88.51-1.08-1.78-.2-3.65-.89-3.65-3.95 0-.87.31-1.59.83-2.15-.08-.2-.36-1.02.08-2.12 0 0 .68-.22 2.2.82A7.67 7.67 0 0 1 8 3.71c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.52.56.83 1.28.83 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.74.54 1.5l-.01 2.32c0 .22.14.47.55.39A8 8 0 0 0 8 0Z" /></svg> : <svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M18.9 2h3.7l-8.1 9.2L24 22h-7.4l-5.8-7.6L4.2 22H.5l8.6-9.8L0 2h7.6l5.2 6.9L18.9 2Zm-1.3 18.1h2L6.5 3.8H4.3l13.3 16.3Z" /></svg>}
    <span>{label}</span>
  </a>;
}

const guideLinks = [
  { href: "https://rlviz.dev", label: "rlviz.dev" },
  { href: "https://github.com/TheSnakeFang/rlviz", label: "Repo" },
  { href: "https://x.com/sofangtastic", label: "Created by Kevin Fang" },
] as const;

function CopyableCode({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    let succeeded = false;
    if (navigator.clipboard?.writeText) {
      try { await navigator.clipboard.writeText(text); succeeded = true; }
      catch { /* Fall through for browsers that expose but restrict Clipboard. */ }
    }
    if (!succeeded) {
      const input = document.createElement("textarea");
      input.value = text;
      input.setAttribute("readonly", "");
      input.style.position = "fixed";
      input.style.opacity = "0";
      document.body.appendChild(input);
      input.select();
      succeeded = document.execCommand("copy");
      input.remove();
    }
    if (!succeeded) return;
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };
  return <div className="guide-code"><pre><code>{text}</code></pre><button type="button" onClick={() => void copy()} aria-label="Copy command">{copied ? "copied" : "copy"}</button></div>;
}

export function Guide({ active, setup, onActivate, onClose }: { active: boolean; setup: ViewerSetup; onActivate: () => void; onClose: () => void }) {
  const [pageID, setPageID] = useState("overview");
  const page = pages.find((candidate) => candidate.id === pageID) ?? pages[0];
  return <article className={`workspace-guide ${active ? "active-zone" : ""}`} tabIndex={0} onFocus={onActivate} onPointerDown={onActivate} aria-label="RLViz guide">
    <header><nav aria-label="RLViz links">{guideLinks.map((link) => <GuideLink key={link.href} {...link} />)}</nav><button onClick={onClose}>close</button></header>
    <div className="guide-layout"><nav aria-label="Guide sections">{pages.map((candidate) => <button key={candidate.id} aria-current={candidate.id === page.id ? "page" : undefined} onClick={() => setPageID(candidate.id)}>{candidate.label}</button>)}</nav>
      <div className="guide-copy">{blocks(page.markdown).map((block, index) => {
        if (block.kind === "h1") return <h1 key={index}>{block.text}</h1>;
        if (block.kind === "h2") return <div key={index}>{page.id === "overview" && setup.mode === "browser" && block.text === "Please read" && <section className="guide-actions" aria-label="Open data"><button className="primary" onClick={setup.onOpenDirectory}>Open trace directory</button><button onClick={setup.onOpenAdapter}>Upload WASM adapter</button></section>}<h2>{block.text}</h2></div>;
        if (block.kind === "li") {
          return <div className="guide-item" key={index}><span>•</span><p><Inline text={block.text} /></p></div>;
        }
        if (block.kind === "code") return <CopyableCode key={index} text={block.text} />;
        return <p key={index}><Inline text={block.text} /></p>;
      })}</div>
    </div>
  </article>;
}
