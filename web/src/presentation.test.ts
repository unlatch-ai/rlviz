import { describe, expect, it } from "vitest";
import { applyPresentationTheme, formatPresentedScalar, presentationDefaultLayout } from "./presentation";
import type { PresentationConfig } from "./types";

describe("presentation configuration", () => {
  it("applies only allowlisted semantic theme tokens and restores inline state", () => {
    const root = document.createElement("div");
    root.style.setProperty("--focus", "#111111");
    const config = {
      api_version: "rlviz.dev/v1alpha1",
      theme: { focus: "#8be6d0", danger: "#ff7580", arbitrary: "#ffffff", warning: "url(https://invalid.example)" },
    } as unknown as PresentationConfig;
    const cleanup = applyPresentationTheme(config, root);
    expect(root.style.getPropertyValue("--focus")).toBe("#8be6d0");
    expect(root.style.getPropertyValue("--danger")).toBe("#ff7580");
    expect(root.style.getPropertyValue("--arbitrary")).toBe("");
    expect(root.style.getPropertyValue("--warning")).toBe("");
    cleanup();
    expect(root.style.getPropertyValue("--focus")).toBe("#111111");
    expect(root.style.getPropertyValue("--danger")).toBe("");
  });

  it("formats the bounded scalar vocabulary deterministically", () => {
    expect(formatPresentedScalar(0.125, { format: "percent_fraction", precision: 1 })).toBe("12.5%");
    expect(formatPresentedScalar(1536, { format: "bytes", precision: 2 })).toBe("1.50 KiB");
    expect(formatPresentedScalar(1250, { format: "duration_ms", precision: 1 })).toBe("1.3s");
    expect(formatPresentedScalar(12.4, { format: "integer", unit: "steps" })).toBe("12 steps");
    expect(formatPresentedScalar(0.0012, { format: "scientific", precision: 2 })).toBe("1.20e-3");
    expect(formatPresentedScalar(1.2, { format: "number" })).toBe("1.2");
  });

  it("derives an exact default layout from configured columns", () => {
    expect(presentationDefaultLayout({ api_version: "rlviz.dev/v1alpha1", group: { columns: ["reward", "signal:grader_score"] } })).toEqual({
      hiddenBuiltins: ["pass", "status", "termination", "events", "errors", "tokens", "latency"],
      signalNames: ["grader_score"],
    });
  });
});
