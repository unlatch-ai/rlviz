export type CohortScalar = string | number | boolean;

export interface CohortQueryRow {
  id: string;
  reward?: number;
  pass?: boolean;
  status?: string;
  termination?: string;
  outcome?: string;
  events?: number;
  errors?: number;
  tokens?: number;
  latency?: number;
  signals: Record<string, CohortScalar>;
}

export type CohortQueryOperator = "=" | "!=" | "<" | "<=" | ">" | ">=";
export type CohortQueryField = "pass" | "status" | "termination" | "outcome" | "reward" | "events" | "errors" | "tokens" | "latency" | `signal.${string}`;

export interface CohortQueryClause {
  field: CohortQueryField;
  operator: CohortQueryOperator;
  value: CohortScalar;
  source: string;
}

export interface CohortQueryDiagnostic {
  code: "unknown-field" | "missing-value" | "invalid-value" | "invalid-operator";
  message: string;
  token: string;
  index: number;
}

export interface ParsedCohortQuery {
  source: string;
  text: string[];
  clauses: CohortQueryClause[];
  diagnostics: CohortQueryDiagnostic[];
}

const stringFields = new Set(["status", "termination", "outcome"]);
const numericFields = new Set(["reward", "events", "errors", "tokens", "latency"]);
const structuredToken = /^([A-Za-z][\w.-]*)(:|!=|<=|>=|=|<|>)(.*)$/;
const finiteNumber = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

function diagnostic(code: CohortQueryDiagnostic["code"], token: string, index: number, message: string): CohortQueryDiagnostic {
  return { code, message, token, index };
}

function parseScalar(value: string): CohortScalar {
  if (value.toLowerCase() === "true") return true;
  if (value.toLowerCase() === "false") return false;
  if (finiteNumber.test(value)) return Number(value);
  return value;
}

function parseStructuredToken(token: string, index: number): { clause?: CohortQueryClause; diagnostic?: CohortQueryDiagnostic } | undefined {
  const match = structuredToken.exec(token);
  if (!match) return undefined;
  const [, rawField, rawOperator, rawValue] = match;
  const field = rawField.toLowerCase();
  if (!rawValue) {
    return { diagnostic: diagnostic("missing-value", token, index, `Missing value after ${rawOperator}.`) };
  }

  const operator = rawOperator === ":" ? "=" : rawOperator as CohortQueryOperator;
  if (field === "pass") {
    if (operator !== "=" && operator !== "!=") {
      return { diagnostic: diagnostic("invalid-operator", token, index, "pass supports only ':' (or '=') and '!='.") };
    }
    const value = rawValue.toLowerCase();
    if (value !== "true" && value !== "false") {
      return { diagnostic: diagnostic("invalid-value", token, index, "pass must be true or false.") };
    }
    return { clause: { field: "pass", operator, value: value === "true", source: token } };
  }

  if (stringFields.has(field)) {
    if (operator !== "=" && operator !== "!=") {
      return { diagnostic: diagnostic("invalid-operator", token, index, `${field} supports only ':' (or '=') and '!='.`) };
    }
    return { clause: { field: field as CohortQueryField, operator, value: rawValue, source: token } };
  }

  if (numericFields.has(field)) {
    if (rawOperator === ":") {
      return { diagnostic: diagnostic("invalid-operator", token, index, `${field} requires a numeric comparison such as ${field}>=1.`) };
    }
    if (!finiteNumber.test(rawValue)) {
      return { diagnostic: diagnostic("invalid-value", token, index, `${field} requires a finite number.`) };
    }
    return { clause: { field: field as CohortQueryField, operator, value: Number(rawValue), source: token } };
  }

  if (field.startsWith("signal.")) {
    const name = rawField.slice("signal.".length);
    if (!name) {
      return { diagnostic: diagnostic("missing-value", token, index, "Signal clauses require a name, for example signal.judge=true.") };
    }
    const value = parseScalar(rawValue);
    if (typeof value !== "number" && operator !== "=" && operator !== "!=") {
      return { diagnostic: diagnostic("invalid-operator", token, index, "String and boolean signals support only ':' (or '=') and '!='.") };
    }
    return { clause: { field: `signal.${name}`, operator, value, source: token } };
  }

  return { diagnostic: diagnostic("unknown-field", token, index, `Unknown cohort field '${rawField}'.`) };
}

/** Parse a deliberately small, whitespace-separated cohort query language. */
export function parseCohortQuery(source: string): ParsedCohortQuery {
  const text: string[] = [];
  const clauses: CohortQueryClause[] = [];
  const diagnostics: CohortQueryDiagnostic[] = [];
  const tokenPattern = /\S+/g;
  for (const match of source.matchAll(tokenPattern)) {
    const token = match[0];
    const parsed = parseStructuredToken(token, match.index);
    if (!parsed) text.push(token);
    else if (parsed.clause) clauses.push(parsed.clause);
    else if (parsed.diagnostic) diagnostics.push(parsed.diagnostic);
  }
  return { source, text, clauses, diagnostics };
}

function compare(actual: CohortScalar | undefined, operator: CohortQueryOperator, expected: CohortScalar): boolean {
  if (actual === undefined || typeof actual !== typeof expected) return false;
  if (typeof actual === "number" && typeof expected === "number") {
    switch (operator) {
      case "=": return actual === expected;
      case "!=": return actual !== expected;
      case "<": return actual < expected;
      case "<=": return actual <= expected;
      case ">": return actual > expected;
      case ">=": return actual >= expected;
    }
  }
  const equal = typeof actual === "string"
    ? actual.toLocaleLowerCase() === String(expected).toLocaleLowerCase()
    : actual === expected;
  return operator === "=" ? equal : operator === "!=" ? !equal : false;
}

function signalValue(row: CohortQueryRow, name: string): CohortScalar | undefined {
  if (Object.hasOwn(row.signals, name)) return row.signals[name];
  const canonical = name.toLocaleLowerCase();
  const entry = Object.entries(row.signals).find(([candidate]) => candidate.toLocaleLowerCase() === canonical);
  return entry?.[1];
}

function clauseValue(row: CohortQueryRow, field: CohortQueryField): CohortScalar | undefined {
  if (field.startsWith("signal.")) return signalValue(row, field.slice("signal.".length));
  switch (field) {
    case "pass": return row.pass;
    case "status": return row.status;
    case "termination": return row.termination;
    case "outcome": return row.outcome;
    case "reward": return row.reward;
    case "events": return row.events;
    case "errors": return row.errors;
    case "tokens": return row.tokens;
    case "latency": return row.latency;
    default: return undefined;
  }
}

function searchableValues(row: CohortQueryRow): string[] {
  return [
    row.id,
    row.reward,
    row.pass,
    row.status,
    row.termination,
    row.outcome,
    row.events,
    row.errors,
    row.tokens,
    row.latency,
    ...Object.entries(row.signals).flatMap(([name, value]) => [name, value]),
  ].filter((value) => value !== undefined).map((value) => String(value).toLocaleLowerCase());
}

/** Invalid structured clauses fail closed; all valid clauses and plain-text terms are ANDed. */
export function matchesCohortQuery(row: CohortQueryRow, query: ParsedCohortQuery): boolean {
  if (query.diagnostics.length) return false;
  if (!query.clauses.every((clause) => compare(clauseValue(row, clause.field), clause.operator, clause.value))) return false;
  const values = searchableValues(row);
  return query.text.every((term) => values.some((value) => value.includes(term.toLocaleLowerCase())));
}
