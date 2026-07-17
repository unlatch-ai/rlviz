import { describe, expect, it } from "vitest";
import { matchesCohortQuery, parseCohortQuery } from "./cohortQuery";
import type { CohortQueryRow } from "./cohortQuery";

const row: CohortQueryRow = {
  id: "traj-alpha",
  reward: 0.75,
  pass: true,
  status: "complete",
  termination: "stop",
  outcome: "solved",
  events: 42,
  errors: 0,
  tokens: 8192,
  latency: 1250,
  signals: { judge: true, difficulty: "Hard", score: 4.5, "grader.label": "safe" },
};

describe("parseCohortQuery", () => {
  it("preserves plain text terms", () => {
    expect(parseCohortQuery("alpha Hard")).toMatchObject({ text: ["alpha", "Hard"], clauses: [], diagnostics: [] });
  });

  it("parses researcher-oriented fields and numeric comparisons", () => {
    const parsed = parseCohortQuery("pass:true status:complete termination!=timeout outcome:solved reward>=0.5 events=42 errors<1 tokens<=9000 latency>1000");
    expect(parsed.diagnostics).toEqual([]);
    expect(parsed.clauses.map(({ field, operator, value }) => ({ field, operator, value }))).toEqual([
      { field: "pass", operator: "=", value: true },
      { field: "status", operator: "=", value: "complete" },
      { field: "termination", operator: "!=", value: "timeout" },
      { field: "outcome", operator: "=", value: "solved" },
      { field: "reward", operator: ">=", value: 0.5 },
      { field: "events", operator: "=", value: 42 },
      { field: "errors", operator: "<", value: 1 },
      { field: "tokens", operator: "<=", value: 9000 },
      { field: "latency", operator: ">", value: 1000 },
    ]);
  });

  it("infers scalar signal values", () => {
    const parsed = parseCohortQuery("signal.judge:true signal.difficulty:Hard signal.score>=4.5");
    expect(parsed.clauses.map((clause) => clause.value)).toEqual([true, "Hard", 4.5]);
  });

  it.each([
    ["wat:value", "unknown-field", "Unknown cohort field 'wat'."],
    ["pass:yes", "invalid-value", "pass must be true or false."],
    ["reward:3", "invalid-operator", "reward requires a numeric comparison such as reward>=1."],
    ["events>=many", "invalid-value", "events requires a finite number."],
    ["status>", "missing-value", "Missing value after >."],
    ["outcome>solved", "invalid-operator", "outcome supports only ':' (or '=') and '!='."],
    ["signal.judge>true", "invalid-operator", "String and boolean signals support only ':' (or '=') and '!='."],
  ])("diagnoses %s", (source, code, message) => {
    expect(parseCohortQuery(source).diagnostics).toEqual([expect.objectContaining({ code, message, token: source, index: 0 })]);
  });
});

describe("matchesCohortQuery", () => {
  it("ANDs plain text terms and structured clauses", () => {
    expect(matchesCohortQuery(row, parseCohortQuery("alpha difficulty pass:true reward>=0.75 errors=0"))).toBe(true);
    expect(matchesCohortQuery(row, parseCohortQuery("alpha missing pass:true"))).toBe(false);
    expect(matchesCohortQuery(row, parseCohortQuery("pass:true reward>0.75"))).toBe(false);
  });

  it("matches strings and signal names case-insensitively", () => {
    expect(matchesCohortQuery(row, parseCohortQuery("status:COMPLETE signal.DIFFICULTY:hard signal.grader.label:safe"))).toBe(true);
  });

  it("supports scalar signal equality and numeric comparisons", () => {
    expect(matchesCohortQuery(row, parseCohortQuery("signal.judge=true signal.score>=4 signal.score<5"))).toBe(true);
    expect(matchesCohortQuery(row, parseCohortQuery("signal.judge!=true"))).toBe(false);
  });

  it("does not treat a missing value as satisfying !=", () => {
    expect(matchesCohortQuery(row, parseCohortQuery("signal.absent!=true"))).toBe(false);
  });

  it("fails closed when a structured-looking clause is malformed", () => {
    expect(matchesCohortQuery(row, parseCohortQuery("alpha reward:high"))).toBe(false);
  });
});
