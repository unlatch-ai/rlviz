import { optionalBuiltinColumns } from "./columnLayout";
import type { GroupColumnLayout } from "./columnLayout";
import { presentationThemeTokens } from "./types";
import type { PresentationConfig, PresentationFieldID, PresentationScalarFormat, PresentationThemeToken } from "./types";

const themeProperties: Record<PresentationThemeToken, `--${string}`> = Object.fromEntries(
  presentationThemeTokens.map((token) => [token, `--${token.replaceAll("_", "-")}`]),
) as Record<PresentationThemeToken, `--${string}`>;

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
  return () => {
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
