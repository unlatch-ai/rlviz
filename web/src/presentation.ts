import { optionalBuiltinColumns } from "./columnLayout";
import type { GroupColumnLayout } from "./columnLayout";
import { presentationInspectorSectionIDs, presentationPaletteTokens, presentationThemeTokens } from "./types";
import type { PresentationConfig, PresentationFieldID, PresentationInspectorSectionID, PresentationPaletteToken, PresentationScalarFormat, PresentationThemeToken } from "./types";

const themeProperties: Record<PresentationThemeToken, `--${string}`> = Object.fromEntries(
  presentationThemeTokens.map((token) => [token, `--${token.replaceAll("_", "-")}`]),
) as Record<PresentationThemeToken, `--${string}`>;

const paletteProperties: Record<PresentationPaletteToken, `--${string}`> = {
  ctx: "--ctx", failPolicy: "--fail-policy", failInfra: "--fail-infra", good: "--good",
  page: "--page", surface: "--surface", ink: "--ink", inkSecondary: "--ink-secondary", muted: "--muted", hairline: "--hairline",
};

const paletteColor = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

/** Applies only the fixed semantic token allowlist and restores prior inline values on cleanup. */
export function applyPresentationTheme(config: PresentationConfig | undefined, root: HTMLElement = document.documentElement): () => void {
  const previous = new Map<string, string>();
  for (const token of presentationThemeTokens) {
    const value = config?.theme?.[token];
    if (!value || !/^#[0-9a-f]{6}$/i.test(value)) continue;
    const property = themeProperties[token];
    previous.set(property, root.style.getPropertyValue(property));
    root.style.setProperty(property, value);
  }

  const palette = config?.palette;
  const paletteIsValid = palette && [palette.light, palette.dark].every((variant) =>
    !variant || Object.entries(variant).every(([token, value]) => presentationPaletteTokens.includes(token as PresentationPaletteToken) && paletteColor.test(value)),
  );
  if (paletteIsValid) {
    for (const property of Object.values(paletteProperties)) previous.set(property, root.style.getPropertyValue(property));
  }
  const darkMedia = typeof window.matchMedia === "function" ? window.matchMedia("(prefers-color-scheme: dark)") : undefined;
  const applyPalette = () => {
    if (!paletteIsValid) return;
    const explicitTheme = root.getAttribute("data-theme");
    const variant = explicitTheme === "dark" || (explicitTheme !== "light" && darkMedia?.matches) ? palette.dark : palette.light;
    for (const token of presentationPaletteTokens) {
      const property = paletteProperties[token];
      const value = variant?.[token];
      if (value) root.style.setProperty(property, value);
      else root.style.removeProperty(property);
    }
  };
  applyPalette();
  darkMedia?.addEventListener("change", applyPalette);
  const observer = paletteIsValid && typeof MutationObserver !== "undefined" ? new MutationObserver(applyPalette) : undefined;
  observer?.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
  return () => {
    observer?.disconnect();
    darkMedia?.removeEventListener("change", applyPalette);
    for (const [property, value] of previous) {
      if (value) root.style.setProperty(property, value);
      else root.style.removeProperty(property);
    }
  };
}

export function presentationDefaultLayout(config?: PresentationConfig): GroupColumnLayout {
  const configured = config?.group?.columns;
  if (!configured?.length) return { hiddenBuiltins: [], signalNames: null };
  const selected = new Set(configured);
  return {
    hiddenBuiltins: optionalBuiltinColumns.filter((column) => !selected.has(column)),
    signalNames: configured.flatMap((column) => column.startsWith("signal:") ? [column.slice(7)] : []),
  };
}

const defaultInspectorSections: PresentationInspectorSectionID[] = [...presentationInspectorSectionIDs];
const allowedInspectorSections = new Set<PresentationInspectorSectionID>(presentationInspectorSectionIDs);

/** Resolve only validated, core-owned inspector primitives; malformed API data fails to defaults. */
export function presentationInspectorSections(config?: PresentationConfig): PresentationInspectorSectionID[] {
  const configured = config?.inspector?.sections;
  if (!configured) return [...defaultInspectorSections];
  if (!configured.length || configured.length > allowedInspectorSections.size || new Set(configured).size !== configured.length || configured.some((section) => !allowedInspectorSections.has(section))) return [...defaultInspectorSections];
  return [...configured];
}

export function fieldMetadata(config: PresentationConfig | undefined, id: PresentationFieldID): { label?: string; description?: string } {
  return config?.fields?.[id] ?? {};
}

export function scalarFormat(config: PresentationConfig | undefined, id: PresentationFieldID): PresentationScalarFormat | undefined {
  return (config?.scalars as Partial<Record<PresentationFieldID, PresentationScalarFormat>> | undefined)?.[id];
}

function decimals(format: PresentationScalarFormat, fallback: number): number {
  return format.precision ?? fallback;
}

function number(value: number, precision: number): string {
  return new Intl.NumberFormat("en-US", { minimumFractionDigits: precision, maximumFractionDigits: precision }).format(value);
}

function generalNumber(value: number, precision?: number): string {
  return precision === undefined
    ? new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(value)
    : number(value, precision);
}

export function formatPresentedScalar(value: string | number | boolean | undefined, format?: PresentationScalarFormat): string {
  if (value === undefined) return "—";
  if (typeof value !== "number" || !format) {
    if (typeof value === "boolean") return value ? "TRUE" : "FALSE";
    return String(value);
  }
  let rendered: string;
  switch (format.format) {
    case "integer": rendered = number(Math.round(value), 0); break;
    case "percent_fraction": rendered = `${number(value * 100, decimals(format, 1))}%`; break;
    case "duration_ms": {
      rendered = value >= 1000 ? `${number(value / 1000, decimals(format, value >= 10_000 ? 1 : 2))}s` : `${number(value, decimals(format, 0))}ms`;
      break;
    }
    case "bytes": {
      const units = ["B", "KiB", "MiB", "GiB", "TiB"];
      let scaled = value; let index = 0;
      while (Math.abs(scaled) >= 1024 && index < units.length - 1) { scaled /= 1024; index += 1; }
      rendered = `${number(scaled, decimals(format, index ? 1 : 0))} ${units[index]}`;
      break;
    }
    case "scientific": rendered = value.toExponential(decimals(format, 3)); break;
    case "number": rendered = generalNumber(value, format.precision); break;
  }
  return format.unit ? `${rendered} ${format.unit}` : rendered;
}
