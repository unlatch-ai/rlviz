export type EventKind =
  | "message"
  | "generation"
  | "tool"
  | "environment_action"
  | "observation"
  | "state"
  | "reward"
  | "grader"
  | "artifact"
  | "error"
  | "log";

export interface SourceLocation {
  path?: string;
  line?: number;
  byte_offset?: number;
  byte_length?: number;
  byte_start?: number;
  byte_end?: number;
}

export type ContextOperation = "compaction" | "truncation" | "injection" | "restore";
export type ContextProvenance = "source_native" | "adapter_derived";

/** Explicit context-window evidence carried by a canonical event. */
export interface EventContext {
  operation?: ContextOperation;
  input_tokens?: number;
  input_tokens_before?: number;
  capacity?: number;
  retained_event_ids?: string[];
  dropped_event_ids?: string[];
  summarized_event_ids?: string[];
  summary?: string;
  provenance: ContextProvenance;
  derivation?: string;
}

export interface TrajectoryEvent {
  id: string;
  sequence: number;
  kind: EventKind | string;
  timestamp?: string;
  title?: string;
  summary?: string;
  content?: unknown;
  data?: unknown;
  input?: unknown;
  output?: unknown;
  metadata?: Record<string, unknown>;
  source?: SourceLocation;
  duration_ms?: number;
  token_count?: number;
  reward?: number;
  parent_id?: string;
  alignment_key?: string;
  context?: EventContext;
  state_hash?: string;
  raw?: unknown;
}

export interface Trajectory {
  id: string;
  name?: string;
  run_id?: string;
  run_name?: string;
  case_id?: string;
  case_name?: string;
  group_id?: string;
  group_name?: string;
  model?: string;
  status?: string;
  termination?: string;
  started_at?: string;
  duration_ms?: number;
  total_reward?: number;
  metadata?: Record<string, unknown>;
  events: TrajectoryEvent[];
  artifacts?: TrajectoryArtifact[];
  signals?: TrajectorySignal[];
}

export interface TrajectorySignal {
  id?: string;
  trajectory_id: string;
  event_id?: string;
  name: string;
  value: unknown;
  unit?: string;
}

export interface TrajectoryArtifact {
  id: string;
  trajectory_id: string;
  event_id?: string;
  name?: string;
  media_type: string;
  path?: string;
  text?: string;
  json?: unknown;
  sha256?: string;
  metadata?: Record<string, unknown>;
}

export interface GroupMetrics {
  reward?: number;
  pass?: boolean;
  success?: boolean;
  outcome?: string;
  event_count?: number;
  error_count?: number;
  token_count?: number;
  duration_ms?: number;
  latency_ms?: number;
  [key: string]: unknown;
}

export interface GroupTrajectorySummary {
  trajectory: (Omit<Trajectory, "events"> & { events?: TrajectoryEvent[] }) | {
    value?: Omit<Trajectory, "events"> & { events?: TrajectoryEvent[] };
    [key: string]: unknown;
  };
  metrics?: GroupMetrics;
  normalized_metrics?: GroupMetrics;
  reward?: number;
  pass?: boolean;
  success?: boolean;
  outcome?: string;
  event_count?: number;
  error_count?: number;
  token_count?: number;
  duration_ms?: number;
  latency_ms?: number;
  signal_count?: number;
  artifact_count?: number;
  status?: string;
  termination?: string;
  signals?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface GroupResponse {
  group_id: string;
  trajectories: GroupTrajectorySummary[];
  aggregates?: Record<string, unknown>;
  count?: number;
  total?: number;
  [key: string]: unknown;
}

export const presentationThemeTokens = [
  "surface_canvas", "surface_panel", "surface_raised", "surface_overlay",
  "border_subtle", "border_strong", "text_primary", "text_secondary",
  "text_muted", "text_faint", "focus", "selection", "success", "info",
  "warning", "danger", "context_change",
] as const;
export type PresentationThemeToken = typeof presentationThemeTokens[number];
export const presentationPaletteTokens = [
  "ctx", "failPolicy", "failInfra", "good", "page", "surface", "ink", "inkSecondary", "muted", "hairline",
] as const;
export type PresentationPaletteToken = typeof presentationPaletteTokens[number];
export type PresentationPaletteVariant = Partial<Record<PresentationPaletteToken, string>>;
export type PresentationFieldID = "reward" | "pass" | "status" | "termination" | "events" | "errors" | "tokens" | "latency" | `signal:${string}`;
export type PresentationScalarFieldID = Exclude<PresentationFieldID, "pass" | "status" | "termination">;
export type PresentationScalarKind = "number" | "integer" | "percent_fraction" | "duration_ms" | "bytes" | "scientific";
export const presentationInspectorSectionIDs = ["properties", "context", "source", "input", "output", "content", "metadata", "linked_artifacts", "analysis", "other_artifacts"] as const;
export type PresentationInspectorSectionID = typeof presentationInspectorSectionIDs[number];

export interface PresentationField {
  label?: string;
  description?: string;
}

export interface PresentationScalarFormat {
  format: PresentationScalarKind;
  precision?: number;
  unit?: string;
}

/** Validated, non-executable presentation metadata from the local daemon. */
export interface PresentationConfig {
  api_version: "rlviz.dev/v1alpha1";
  fields?: Partial<Record<PresentationFieldID, PresentationField>>;
  scalars?: Partial<Record<PresentationScalarFieldID, PresentationScalarFormat>>;
  group?: { columns?: PresentationFieldID[] };
  inspector?: { sections?: PresentationInspectorSectionID[] };
  keymap?: { bindings?: Record<string, string[]> };
  theme?: Partial<Record<PresentationThemeToken, string>>;
  palette?: { name?: "high-contrast"; light?: PresentationPaletteVariant; dark?: PresentationPaletteVariant };
  notices?: string[];
}

export interface PathFingerprint {
  kind: string;
  class: string;
  alignment_key?: string;
  state_hash?: string;
  digest?: string;
  behavioral: boolean;
}

export interface GroupPathNode {
  fingerprint: PathFingerprint;
  count: number;
  terminal_count: number;
  trajectory_ids: string[];
  trajectory_ids_truncated?: boolean;
  narrative_event_count: number;
  depth: number;
  children: GroupPathNode[];
}

export interface GroupPathTree {
  trajectory_count: number;
  terminal_count: number;
  narrative_only_count: number;
  behavioral_event_count: number;
  narrative_event_count: number;
  root_narrative_event_count: number;
  children: GroupPathNode[];
}

export interface GroupPathsResponse {
  group_id: string;
  tree: GroupPathTree;
  source_native_branches: boolean;
  source_native_branch_count: number;
  count: number;
  total_events: number;
}

export interface TrajectoryResponse {
  trajectory?: Omit<Trajectory, "events"> & { events?: TrajectoryEvent[] };
  events?: TrajectoryEvent[];
  run?: { id: string; name?: string; started_at?: string; metadata?: Record<string, unknown> };
  case?: { id: string; run_id: string; name?: string; metadata?: Record<string, unknown> };
  group?: { id: string; case_id: string; name?: string; metadata?: Record<string, unknown> };
  signals?: TrajectorySignal[];
  artifacts?: TrajectoryArtifact[];
  signal_page?: PageMetadata;
  artifact_page?: PageMetadata;
  source?: IndexedSource;
  presentation?: PresentationConfig;
  id?: string;
  name?: string;
  [key: string]: unknown;
}

export interface IndexedSource {
  id: string;
  path?: string;
  index_state?: "indexing" | "refreshing" | "complete" | "failed" | string;
  index_error?: string;
}

export interface BrowseTrajectory {
  source_id: string;
  source_name: string;
  run_name?: string;
  case_name?: string;
  group_name?: string;
  trajectory: Omit<Trajectory, "events">;
  metrics: BrowseMetrics;
  /** Compact truthful shape summary (see instrument.summarizeShape). Absent
   * until the provider supplies it; renderers must not fabricate texture. */
  shape?: { events: number; slots: Array<{ count: number; tools: number; landmark?: "error" | "context" | "evidence" }> };
}

export interface BrowseMetrics extends GroupMetrics {
  trajectory?: GroupTrajectorySummary["trajectory"];
  metrics?: GroupMetrics;
  normalized_metrics?: GroupMetrics;
  signals?: Record<string, unknown>;
  signal_count?: number;
  artifact_count?: number;
  status?: string;
  termination?: string;
}

export interface BrowseResponse {
  sources: IndexedSource[];
  trajectories: BrowseTrajectory[];
  count: number;
}

export interface PageMetadata {
  count: number;
  total: number;
  limit: number;
  after_sequence?: number;
  next_sequence?: number;
  has_more: boolean;
  offset?: number;
  next_offset?: number;
}

export interface EventPageResponse {
  events: TrajectoryEvent[];
  page: PageMetadata;
  source?: IndexedSource;
}

export interface ChildPageResponse<T> {
  page: PageMetadata;
  signals?: T[];
  artifacts?: T[];
}

export type AlignmentOperation = "match" | "replace" | "delete" | "insert";

export interface AlignmentFingerprint {
  kind: string;
  class: string;
  alignment_key?: string;
  state_hash?: string;
  digest?: string;
  behavioral: boolean;
}

export interface AlignmentStep {
  operation: AlignmentOperation;
  left_index?: number;
  right_index?: number;
  left?: AlignmentFingerprint;
  right?: AlignmentFingerprint;
  meaningful: boolean;
}

export interface ComparisonSide {
  trajectory: Omit<Trajectory, "events">;
  events: TrajectoryEvent[];
  signals?: Array<{ name: string; value: unknown }>;
  artifacts?: TrajectoryArtifact[];
}

export interface ValueDifference {
  left?: unknown;
  right?: unknown;
  changed: boolean;
}

export interface CountDifference {
  left: number;
  right: number;
  delta: number;
}

export interface NumericDifference extends ValueDifference {
  left?: number;
  right?: number;
  delta?: number;
}

export interface VerifierResult {
  event_id: string;
  sequence: number;
  alignment_key?: string;
  output?: unknown;
}

export interface VerifierDifference {
  left?: VerifierResult[];
  right?: VerifierResult[];
  changed: boolean;
}

export interface ComparisonResponse {
  left: ComparisonSide;
  right: ComparisonSide;
  alignment: {
    steps: AlignmentStep[];
    common_behavioral_prefix: number;
    first_meaningful_divergence?: number;
    later_realignment?: number;
  };
  differences: {
    event_count: CountDifference;
    status: ValueDifference;
    termination: ValueDifference;
    reward: ValueDifference;
    success?: ValueDifference;
    token_count?: NumericDifference;
    context_event_count?: CountDifference;
    compaction_count?: CountDifference;
    verifier_results?: VerifierDifference;
  };
}

export interface AnalyzerProvenance {
  name: string;
  version: string;
  digest: string;
  input_digest: string;
}

export interface AnalyzerFinding {
  id: string;
  trajectory_id: string;
  event_ids?: string[];
  kind: string;
  severity: "info" | "warning" | "error";
  title: string;
  summary?: string;
  fingerprint?: string;
  metadata?: Record<string, unknown>;
}

export interface AnalyzerSignal {
  id: string;
  trajectory_id: string;
  event_id?: string;
  name: string;
  value: unknown;
  unit?: string;
  metadata?: Record<string, unknown>;
}

export interface AnalysisResponse {
  analysis: {
    api_version: string;
    provenance: AnalyzerProvenance;
    findings?: AnalyzerFinding[];
    signals?: AnalyzerSignal[];
  };
  cached: boolean;
  analyzed_at: string;
}
