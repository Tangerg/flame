import {
  type Item,
  type PendingInterruptSet,
  type RunProtocolProfile,
  type RunRef,
  type StreamEvent,
} from "@flame/runtime-client";
import { type AgentEventEnvelope } from "@/features/agent/public/events";
import {
  runtimeAgentEvent,
  runtimeItem,
  runtimePendingInterruptSet,
  runtimeRunFact,
} from "@/features/agent/adapters/runtimeAgentFacts";
import { type AgentSessionSnapshot } from "@/features/agent/application/ports/runtimeGateway";
import { toolCategoryForName } from "@/features/agent/application/fold/reducer.fixtures";
import { type GoalReadModel } from "@/features/goal/application/goalReadModel";
import { VISUAL_CONTEXT_TOKENS } from "./agentFixtureFacts";
export const VISUAL_AGENT_STATES = [
  "empty",
  "runtime-down",
  "idle",
  "running",
  "answer-opening",
  "steer",
  "waiting",
  "question",
  "terminal",
  "canceled",
  "error",
  "error-retryable",
  "recovery",
  "cwd-missing",
  "delegated",
  "long-content",
  "prose-blocks",
  "code-reading",
  "table-reading",
  "reference-notes",
  "math-content",
  "callouts",
  "narrative",
  "tool-shells",
  "tool-search",
  "tool-remote",
  "tool-agentic",
  "tool-tail",
  "question-multi",
  "waves",
] as const;

export type VisualAgentState = (typeof VISUAL_AGENT_STATES)[number];

const SESSION_ID = "ses_visual";

const ROOT_RUN_ID = "run_root";

const CREATED_AT = "2026-07-31T08:00:00.000Z";

const RUN_PROVIDER = "openai";

const RUN_MODEL = "gpt-5.6-sol";

const METRICS = {
  steps: 4,
  activeDurationMillis: 12_000,
  usage: { inputTokens: 96_000, outputTokens: 3_100, cacheReadTokens: 64_000 },
};

const PROFILE: RunProtocolProfile = {
  interruptTypes: ["approval"],
  requiredFeatures: ["subagents"],
};

export type RuntimeAgentSessionSnapshot = Omit<
  AgentSessionSnapshot,
  "items" | "pendingInterruptSets" | "runs"
> & {
  items: Item[];
  pendingInterruptSets: PendingInterruptSet[];
  runs: RunRef[];
};

const SAFETY_CLASS: Record<string, "safe" | "write" | "exec" | "network"> = {
  read: "safe",
  grep: "safe",
  glob: "safe",
  lsp: "safe",
  inspect_workspace: "safe",
  read_shell_output: "safe",
  list_schedules: "safe",
  list_skills: "safe",
  load_skill: "safe",
  read_skill_resource: "safe",
  propose_skill: "safe",
  search_memory: "safe",
  search_tools: "safe",
  read_tool_result: "safe",
  ask_user: "safe",
  enter_plan_mode: "safe",
  set_plan: "safe",
  delegate_task: "safe",
  get_goal: "safe",
  report_goal_outcome: "safe",
  apply_patch: "write",
  shell: "exec",
  stop_shell: "exec",
  web_search: "network",
  web_fetch: "network",
  http_request: "network",
};

function safetyClassOf(name: string): "safe" | "write" | "exec" | "network" {
  const safetyClass = SAFETY_CLASS[name];
  if (safetyClass === undefined) {
    throw new Error(`no Runtime safety class recorded for the tool '${name}'`);
  }
  return safetyClass;
}

function run(status: RunRef["status"], patch: Partial<RunRef> = {}): RunRef {
  return {
    id: ROOT_RUN_ID,
    sessionId: SESSION_ID,
    status,
    createdAt: CREATED_AT,
    metrics: METRICS,
    protocolProfile: PROFILE,
    provider: RUN_PROVIDER,
    model: RUN_MODEL,
    ...(status === "running" ? { activeSegmentId: "seg_root" } : {}),
    ...patch,
  };
}

function message(
  type: "userMessage" | "agentMessage",
  id: string,
  text: string,
  runId = ROOT_RUN_ID,
  phase: "commentary" | "finalAnswer" = "finalAnswer",
): Item {
  const facts = {
    id,
    runId,
    status: "completed" as const,
    createdAt: CREATED_AT,
    content: [{ type: "text" as const, text }],
  };
  return type === "agentMessage" ? { type, phase, ...facts } : { type, ...facts };
}

const PROMPT = message(
  "userMessage",
  "item_prompt",
  "Review the Runtime boundary and keep application-owned atomicity out of the Agent Framework.",
);

const RESPONSE = message(
  "agentMessage",
  "item_response",
  "The boundary is clean: the framework exposes execution primitives, while the application owns persistence, idempotency, and transaction policy.",
);

const COMMENTARY_RESPONSE = message(
  "agentMessage",
  "item_response",
  "The boundary is clean: the framework exposes execution primitives, while the application owns persistence, idempotency, and transaction policy.",
  ROOT_RUN_ID,
  "commentary",
);

const RUNNING_RESPONSE: Item = {
  type: "agentMessage",
  id: "item_running_response",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  content: [{ type: "text", text: "I’m tracing the ownership boundary and verifying" }],
};

const OPENING_RESPONSE: Item = {
  type: "agentMessage",
  id: "item_running_response",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  content: [],
};

const RUNNING_REASONING: Item = {
  type: "reasoning",
  id: "item_reasoning",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  text: [
    "The framework must expose execution capability without knowing the application’s persistence records.",
    "That boundary is the whole reason a Run can be replayed: the framework owns the step, the application owns what a step MEANT.",
    "If the framework knew about records it would have to know about transactions, and then about idempotency keys, and then it would be the application.",
    "So the seam is drawn where the knowledge stops rather than where the call stack happens to be.",
    "Which leaves one question open — who decides a step is finished — and the answer has to be the side that can durably say so.",
    "A step the framework calls finished is a claim; a step the application has written down is a fact, and only the second one survives a restart.",
    "So the protocol carries the claim and the store carries the fact, and recovery is the act of reconciling the two in that order.",
    "Everything else in this boundary follows from that ordering, including why an interrupt has to be durable before it is acknowledged.",
    "Which also settles the compaction question: a summary is application material, so the framework may ask for one but must never assume it happened.",
    "The same reasoning applies to a Goal, whose lifecycle is a durable fact and whose progress is a claim.",
    "A Plan is the same shape one level down: its steps are claims the model makes, and only the Runtime can mark one done because only the Runtime watched it happen.",
    "That is also why a steer arrives as an item rather than as an edit — an edit would rewrite a fact, and an item adds one.",
  ].join(" "),
};

const RUNNING_SET_PLAN: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_running_set_plan",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 12,
  finishedAt: "2026-07-31T08:00:00.012Z",
  tool: {
    name: "set_plan",
    arguments: {
      steps: ["Verify boundary ownership", "Review visual evidence", "Run quality gates"],
    },
  },
};

const READ_STORE_RESULT = {
  content: [
    "package session",
    "",
    "func (s *Store) Commit(ctx context.Context, session *Session, records []Record, opts CommitOptions) (Receipt, error) {",
    "\tif err := s.flushLocked(); err != nil {",
    '\t\treturn Receipt{}, fmt.Errorf("commit: %w", err)',
    "\t}",
  ].join("\n"),
  start_line: 1,
  end_line: 6,
  total_lines: 6,
};

const RUNNING_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_running_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 36,
  finishedAt: "2026-07-31T08:00:00.036Z",
  tool: {
    name: "read",
    arguments: {
      path: "/Users/visual/scope/runtime/internal/session/atomicity_and_idempotency.go",
    },
    result: READ_STORE_RESULT,
  },
};

const RUNNING_TOOL: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_running_tool",
  runId: ROOT_RUN_ID,
  status: "running",
  startedAt: CREATED_AT,
  tool: {
    name: "grep",
    arguments: {
      pattern: "idempotency|atomicity",
      path: "runtime",
    },
  },
};

const PENDING_APPROVAL_TOOL: Item = {
  type: "toolCall",
  safetyClass: "exec",
  id: "item_approval",
  runId: ROOT_RUN_ID,
  status: "running",
  startedAt: CREATED_AT,
  tool: {
    name: "shell",
    arguments: {
      description: "Run the race detector",
      command: "go test -race ./...",
    },
  },
};

function settledTool(
  id: string,
  name: string,
  args: Record<string, unknown>,
  result: unknown,
): Item {
  return {
    type: "toolCall",
    safetyClass: safetyClassOf(name),
    id,
    runId: ROOT_RUN_ID,
    status: "completed",
    startedAt: CREATED_AT,
    durationMillis: 120,
    finishedAt: "2026-07-31T08:00:00.120Z",
    tool: { name, category: toolCategoryForName(name), arguments: args, result },
  };
}

const GLOB_CALL = settledTool(
  "item_search_glob",
  "glob",
  { pattern: "runtime/**/*_test.go" },
  JSON.stringify({
    hits: [
      { path: "runtime/internal/session/store_test.go" },
      { path: "runtime/internal/session/atomicity_test.go" },
      { path: "runtime/internal/run/segment_test.go" },
    ],
  }),
);

const MEMORY_CALL = settledTool(
  "item_search_memory",
  "search_memory",
  { query: "compaction cutpoint" },
  [
    "1. The compaction cutpoint is chosen by the Runtime, never by the client.",
    "2. A steer arriving during compaction is queued, not dropped.",
  ].join("\n"),
);

const TOOL_SEARCH_CALL = settledTool(
  "item_search_tools",
  "search_tools",
  { query: "schedule" },
  [
    "Not loaded:",
    "  [flame] create_schedule, list_schedules, delete_schedule",
    "  [mcp:github] create_issue",
  ].join("\n"),
);

const WEB_SEARCH_CALL = settledTool(
  "item_remote_web_search",
  "web_search",
  { query: "sqlite wal checkpoint starvation" },
  JSON.stringify({
    results: [
      {
        url: "https://www.sqlite.org/wal.html",
        title: "Write-Ahead Logging",
        snippet: "A checkpoint runs automatically when the WAL passes a threshold.",
      },
      {
        url: "https://sqlite.org/forum/forumpost/2c7b9f",
        title: "WAL grows without bound under a long reader",
        snippet: "A reader open across the checkpoint keeps frames alive.",
      },
    ],
  }),
);

const WEB_FETCH_CALL = settledTool(
  "item_remote_web_fetch",
  "web_fetch",
  { url: "https://www.sqlite.org/wal.html" },
  JSON.stringify({
    format: "markdown",
    content:
      "# Write-Ahead Logging\n\nA checkpoint moves frames from the WAL back into the database.",
  }),
);

const HTTP_CALL = settledTool(
  "item_remote_http",
  "http_request",
  { method: "GET", url: "http://127.0.0.1:17171/v2/health/ready" },
  JSON.stringify({
    status: 200,
    duration: "12ms",
    truncated: false,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
    body: '{"ready":true,"database":"open"}',
  }),
);

const SCHEDULES_CALL = settledTool(
  "item_remote_schedules",
  "list_schedules",
  {},
  JSON.stringify({
    schedules: [
      {
        schedule_id: "sch_nightly",
        title: "Nightly conformance sweep",
        cron: "0 3 * * *",
        instructions: "Run the conformance gates and post the digest.",
        enabled: true,
        next_run_at: "2026-08-01T03:00:00Z",
        last_run_at: "2026-07-31T03:00:00Z",
      },
      {
        schedule_id: "sch_weekly",
        title: "Weekly dependency audit",
        cron: "0 9 * * 1",
        instructions: "Check for released framework versions.",
        enabled: false,
        next_run_at: "",
        last_run_at: "2026-07-27T09:00:00Z",
      },
    ],
  }),
);

const LIST_SKILLS_CALL = settledTool(
  "item_agentic_list_skills",
  "list_skills",
  {},
  [
    "<available_skills>",
    "<skill><name>review-diff</name><description>Read a change the way a reviewer does, worst risk first.</description></skill>",
    "<skill><name>trace-flaky-test</name><description>Find the shared state behind a test that passes alone.</description></skill>",
    "</available_skills>",
  ].join("\n"),
);

const LOAD_SKILL_CALL = settledTool(
  "item_agentic_load_skill",
  "load_skill",
  { name: "review-diff" },
  [
    "Start from the riskiest hunk, not the first one.",
    "Name the invariant the change could break, then look for the test that would catch it.",
    "A diff with no test change is a claim that nothing observable changed. Check it.",
  ].join("\n"),
);

const ENTER_PLAN_CALL = settledTool(
  "item_agentic_enter_plan",
  "enter_plan_mode",
  {},
  "Planning only from here: no edits until the plan is accepted.",
);

const GET_GOAL_CALL = settledTool(
  "item_agentic_goal",
  "get_goal",
  {},
  JSON.stringify({
    goal: {
      objective: "Keep application-owned atomicity out of the Agent Framework.",
      status: "active",
    },
  }),
);

const REPORT_GOAL_CALL = settledTool(
  "item_agentic_report_goal",
  "report_goal_outcome",
  { outcome: "achieved" },
  "Goal achieved: the boundary holds.",
);

const READ_SHELL_CALL = settledTool(
  "item_agentic_read_shell",
  "read_shell_output",
  { shell_id: "sh_01" },
  [
    "waiting for the build to settle",
    "ok  \tgithub.com/Tangerg/flame/runtime/internal/run\t2.104s",
  ].join("\n"),
);

const LSP_CALL = settledTool(
  "item_agentic_lsp",
  "lsp",
  { operation: "references", path: "runtime/internal/session/store.go", line: 214, character: 6 },
  [
    "runtime/internal/session/store.go:214:6",
    "runtime/internal/session/atomicity.go:88:14",
    "runtime/internal/run/segment.go:41:9",
  ].join("\n"),
);

const WORKSPACE_INSPECTION_CALL = settledTool(
  "item_agentic_inspect_workspace",
  "inspect_workspace",
  {
    workspace: {
      root: `/Users/visual/Flame workspaces/${"application-boundary-".repeat(12)}review`,
      include: ["runtime/internal/session/**", "apps/web/src/**"],
    },
    checks: ["ownership", "dependency-direction"],
    notes: "Keep <component> tags and source paths as literal text.",
  },
  JSON.stringify({
    summary: { status: "reviewed", files: 24 },
    files: Array.from({ length: 24 }, (_, index) => ({
      path: `runtime/internal/session/state_${index + 1}.go`,
      status: "reviewed",
    })),
  }),
);

const PROPOSE_SKILL_CALL = settledTool(
  "item_last_propose_skill",
  "propose_skill",
  { name: "trace-flaky-test" },
  "Proposed trace-flaky-test for review. It will not load until someone approves it.",
);

const SKILL_RESOURCE_CALL = settledTool(
  "item_last_skill_resource",
  "read_skill_resource",
  { name: "review-diff", path: "checklist.md" },
  ["- Riskiest hunk first.", "- Name the invariant.", "- Find the test that would catch it."].join(
    "\n",
  ),
);

const READ_TOOL_RESULT_CALL = settledTool(
  "item_last_read_tool_result",
  "read_tool_result",
  { item_id: "item_shells_command" },
  ["ok  \tgithub.com/Tangerg/flame/runtime/internal/session\t8.412s", "FAIL"].join("\n"),
);

const STOP_SHELL_CALL = settledTool(
  "item_last_stop_shell",
  "stop_shell",
  { shell_id: "sh_01" },
  "Stopped sh_01 after 2.1s.",
);

const ASK_USER_CALL = settledTool(
  "item_last_ask_user",
  "ask_user",
  { questions: [{ question: "Which gate should run next?", header: "Next gate" }] },
  "Race detector",
);

const SHELL_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_shells_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 42,
  finishedAt: "2026-07-31T08:00:00.042Z",
  tool: {
    name: "read",
    arguments: { path: "runtime/internal/session/store.go" },
    result: READ_STORE_RESULT,
  },
};

const SHELL_COMMAND: Item = {
  type: "toolCall",
  safetyClass: "exec",
  id: "item_shells_command",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 8400,
  finishedAt: "2026-07-31T08:00:08.400Z",
  tool: {
    name: "shell",
    arguments: {
      description: "Run the session suite",
      command: "go test ./internal/session/...",
    },
    result: {
      exitCode: 1,
      output: [
        "\u001b[1m=== RUN   TestCommitAtomicity\u001b[0m",
        "\u001b[32m--- PASS: TestCommitAtomicity (0.01s)\u001b[0m",
        "=== RUN   TestCommitIdempotency",
        "\u001b[32m--- PASS: TestCommitIdempotency (0.00s)\u001b[0m",
        "=== RUN   TestRollbackOnFlushFailure",
        "    store_test.go:214: expected rollback, got commit",
        "\u001b[31m--- FAIL: TestRollbackOnFlushFailure (0.02s)\u001b[0m",
        "\u001b[33mwarning: 1 test skipped\u001b[0m",
        "\u001b[31mFAIL\u001b[0m\tgithub.com/Tangerg/flame/runtime/internal/session\t8.412s",
        "FAIL",
      ].join("\n"),
    },
  },
};

// Abbreviated patch inputs have no parseable line counts; these captured
// receipts retain the Runtime presenter's explicit zero counts.
const PATCH_NEW_FILE: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_write",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 40,
  finishedAt: "2026-07-31T08:00:00.040Z",
  tool: {
    name: "apply_patch",
    arguments: { patch: "*** Begin Patch\n*** Add File: atomicity.md\n…\n*** End Patch" },
    result: {
      changes: [
        { path: "runtime/internal/session/atomicity.md", status: "added", added: 0, removed: 0 },
      ],
    },
  },
};

const SHELL_FAILED: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_failed",
  runId: ROOT_RUN_ID,
  status: "incomplete",
  startedAt: CREATED_AT,
  durationMillis: 120,
  finishedAt: "2026-07-31T08:00:00.120Z",
  tool: { name: "apply_patch", arguments: { patch: "*** Begin Patch\n…\n*** End Patch" } },
  error: { type: "tool_failed", detail: "store.go changed on disk after it was read." },
};

const SHELL_PATCH: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_edit",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 64,
  finishedAt: "2026-07-31T08:00:00.064Z",
  tool: {
    name: "apply_patch",
    arguments: { patch: "*** Begin Patch\n…\n*** End Patch" },
    result: {
      changes: [
        {
          path: "/Users/visual/scope/apps/web/src/features/chat/tools/application/specialisedPreviewProjections.ts",
          status: "modified",
          added: 0,
          removed: 0,
        },
        {
          path: "runtime/internal/session/atomicity_and_idempotency.go",
          status: "moved",
          from: "runtime/internal/session/store_atomicity.go",
          added: 0,
          removed: 0,
        },
        {
          path: "runtime/internal/session/legacy_store_shim.go",
          status: "deleted",
          added: 0,
          removed: 0,
        },
      ],
    },
  },
};

const SHELL_DENIED: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_denied",
  runId: ROOT_RUN_ID,
  status: "incomplete",
  startedAt: CREATED_AT,
  durationMillis: 15,
  finishedAt: "2026-07-31T08:00:00.015Z",
  tool: { name: "apply_patch", arguments: { patch: "*** Begin Patch\n…\n*** End Patch" } },
  error: { type: "denied_by_user", detail: "You declined this write." },
};

const WAVE_REASONING_ONE: Item = {
  type: "reasoning",
  id: "item_w_reason_1",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: CREATED_AT,
  text: "Find where the boundary is enforced before changing anything.",
};

const WAVE_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_w_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 31,
  finishedAt: "2026-07-31T08:00:00.031Z",
  tool: {
    name: "read",
    arguments: { path: "runtime/internal/session/store.go" },
    result: READ_STORE_RESULT,
  },
};

const WAVE_GREP: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_w_grep",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 88,
  finishedAt: "2026-07-31T08:00:00.088Z",
  tool: {
    name: "grep",
    arguments: { pattern: "RunInTx", path: "runtime" },
    result: {
      hits: [
        {
          path: "runtime/internal/session/store.go",
          lineNumber: 88,
          snippet: "func (s *Store) RunInTx(ctx context.Context) error {",
        },
        {
          path: "runtime/internal/run/segment.go",
          lineNumber: 143,
          snippet: "\treturn s.store.RunInTx(ctx)",
        },
      ],
    },
  },
};

const WAVE_ANSWER_ONE = message(
  "agentMessage",
  "item_w_answer_1",
  "The transaction seam is `RunInTx`, and every store call already goes through it.",
  ROOT_RUN_ID,
  "commentary",
);

const WAVE_REASONING_TWO: Item = {
  type: "reasoning",
  id: "item_w_reason_2",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: CREATED_AT,
  text: "So the change is one call site, plus a test that proves the rollback.",
};

const WAVE_PATCH: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_w_edit",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  durationMillis: 57,
  finishedAt: "2026-07-31T08:00:00.057Z",
  tool: {
    name: "apply_patch",
    arguments: { patch: "*** Begin Patch\n…\n*** End Patch" },
    result: {
      changes: [
        { path: "runtime/internal/session/store.go", status: "modified", added: 0, removed: 0 },
      ],
    },
  },
};

const WAVE_ANSWER_TWO = message(
  "agentMessage",
  "item_w_answer_2",
  "Done — the write now rolls back with the transaction.",
  ROOT_RUN_ID,
  "commentary",
);

const WAVE_LIVE_REASONING: Item = {
  type: "reasoning",
  id: "item_w_reason_3",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  text: "Now check whether the test suite covers the rollback path.",
};

const WAVE_LIVE_TOOL: Item = {
  type: "toolCall",
  safetyClass: "exec",
  id: "item_w_live",
  runId: ROOT_RUN_ID,
  status: "running",
  startedAt: CREATED_AT,
  tool: {
    name: "shell",
    arguments: {
      description: "Run the session suite",
      command: "go test ./internal/session/...",
    },
  },
};

interface TailFrame {
  index: number;
  event: StreamEvent;
}

function tail(index: number, event: StreamEvent): TailFrame {
  return { index, event };
}

function tailEvent(index: number, event: StreamEvent): AgentEventEnvelope {
  return runtimeAgentEvent({
    event,
    eventId: `event_visual_${index}`,
    runId: ROOT_RUN_ID,
    segmentId: "seg_root",
    timestamp: CREATED_AT,
  });
}

export const LONG_RESPONSE_MARKDOWN = [
  "## Architecture review",
  "",
  "The consumer owns persistence policy and transaction scope. The Agent Framework remains reusable because it exposes execution capability without importing application records.",
  "",
  "The implementation is in runtime/internal/session/store.go:4. Inspect the source, then return to this reading position.",
  "",
  "- keep Run and Item protocol facts at the application boundary;",
  "- keep framework identities opaque;",
  "- project durable state atomically before publishing it;",
  "- reject compatibility aliases during development.",
  "",
  "| Boundary | Owner | Checks |",
  "|---|---|---:|",
  "| Run lifecycle | Application | 18 |",
  "| Execution capability | Framework | 7 |",
  "",
  "![Inline architecture](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNDAgOTYiIHdpZHRoPSIyNDAiIGhlaWdodD0iOTYiPjxyZWN0IHdpZHRoPSIyNDAiIGhlaWdodD0iOTYiIHJ4PSIxNiIgZmlsbD0iI2U4ZWVmYyIvPjxjaXJjbGUgY3g9IjEyMCIgY3k9IjQ4IiByPSIyMiIgZmlsbD0iIzM1NzRmMCIvPjwvc3ZnPg==) ![Inline detail](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNDAgOTYiIHdpZHRoPSIyNDAiIGhlaWdodD0iOTYiPjxyZWN0IHdpZHRoPSIyNDAiIGhlaWdodD0iOTYiIHJ4PSIxNiIgZmlsbD0iI2U4ZWVmYyIvPjxjaXJjbGUgY3g9IjEyMCIgY3k9IjQ4IiByPSIyMiIgZmlsbD0iIzM1NzRmMCIvPjwvc3ZnPg==)",
  "",
  "![Tracking pixel](https://tracker.example/pixel.png)",
  "",
  "```go",
  "type Executor interface {",
  "    Execute(context.Context, Request) (Result, error)",
  "}",
  "```",
  "",
  "```",
  "npm run check",
  "```",
  "",
  "```html",
  '<!doctype html><html><body><script>parent.postMessage("ran", "*")</script></body></html>',
  "```",
  "",
  "```mermaid",
  "graph LR",
  "  Runtime --> Desktop",
  "  Desktop --> Frontend",
  "```",
  "",
  "```svg",
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 96" width="240" height="96">',
  '  <rect width="240" height="96" rx="16" fill="#e8eefc"/>',
  '  <path d="M28 48h72m40 0h72" stroke="#3574f0" stroke-width="8" stroke-linecap="round"/>',
  '  <circle cx="120" cy="48" r="22" fill="#3574f0"/>',
  "</svg>",
  "```",
  "",
  "### Where the boundary is enforced",
  "",
  "A deliberately long final paragraph verifies wrapping, reading measure, CJK fallback（中文混排）, inline code such as `expectedRuntimeProjectionRevisionIdentifierWithoutSoftBreaksAcrossTheCompleteCodexReadingMeasureAndEveryContinuationBoundary`, and uninterrupted vertical rhythm without inventing a fixture-only message shape.",
  "",
  "### اتجاه القائمة",
  "",
  "- المرحلة الأولى",
  "- المرحلة الثانية",
  "",
  "### Structural primitives",
  "",
  "> Ownership stays with the application boundary.",
  "",
  "1. [x] Preserve the durable fact",
  "",
  "   Keep the follow-up explanation aligned with the task body, not beneath the checkbox.",
  "2. [ ] Verify the projected view",
  "",
  "- Primary marker",
  "    - Nested marker",
  "        - Deep marker",
  "",
  "---",
  "",
  "The structure settles back into prose.",
  "",
  "### 中文长回答",
  "",
  "这次调整从阅读体验出发：标题负责划分层级，段落负责组织一个完整的观点。正文、列表和工具输出沿用同一条内容边界，让视线可以自然地向下移动，不必反复寻找下一段从哪里开始。",
  "",
  "代码片段如 `runtime.Commit` 应该自然地融入句子；路径和较长的参数可以完整查看，也能直接复制。表格、图表和展开的执行记录都服务于回答本身，保留必要的细节，同时避免过重的边框和重复的标题。",
  "",
  "完成检查后，阅读者应当能够快速判断结论、证据与下一步操作。即使切换到深色主题、放大字号，或者同时打开右侧工作区，这些层级和间距也应保持清晰。",
  "",
  "#### Verification details",
  "",
  "Inspect the captured result before returning to the conversation.",
  "",
  "##### Keyboard and focus",
  "",
  "Keep the selected material reachable without losing the original reading position.",
  "",
  "###### Narrow reading views",
  "",
  "长标题和正文应保持明确的间隔，嵌套层级依靠字号、字重和留白共同表达。",
].join("\n");

const LONG_RESPONSE = message("agentMessage", "item_long_response", LONG_RESPONSE_MARKDOWN);

export const CODE_READING_SOURCE = Array.from(
  { length: 48 },
  (_, index) =>
    `const observation${String(index + 1).padStart(2, "0")} = { owner: "Runtime", accepted: true };`,
).join("\n");

const CODE_READING_MARKDOWN = [
  "## Captured boundary review",
  "",
  "Read the accepted observations, then continue with the conclusion.",
  "",
  "```typescript",
  CODE_READING_SOURCE,
  "```",
  "",
  "The explanation continues with the same reading rhythm.",
].join("\n");

const TABLE_READING_MARKDOWN = [
  "Compare the complete recorded observations.",
  "",
  "| Observation | Owner | Checks |",
  "| --- | --- | ---: |",
  ...Array.from(
    { length: 80 },
    (_, index) =>
      `| Observation ${index + 1} | Application boundary for recorded observation | ${index + 1} |`,
  ),
  "",
  "The explanation continues after the recorded evidence.",
].join("\n");

export const READING_BLOCKS_MARKDOWN = [
  "## Reading notes",
  "",
  "The review keeps observations, evidence and follow-up work in a clear reading order.",
  "",
  "### Quoted evidence",
  "",
  "> The Runtime owns the durable facts and their transitions.",
  ">",
  "> The client renders those facts without advancing a second state machine.",
  "",
  "### Evidence and next steps",
  "",
  "> Before publishing a result, retain the evidence used to reach it.",
  ">",
  "> - Keep the accepted read with its requested file.",
  "> - Verify the consumer of the public contract.",
  ">",
  "> Continue only after the recorded result can be explained.",
  "",
  "### Nested context",
  "",
  "> Preserve the original observation in the surrounding review.",
  ">",
  "> > 中文引用也需要保留完整的段落边界。",
  "> >",
  "> > 后续说明应当自然地衔接，不让两段文字粘在一起。",
  ">",
  "> The surrounding review then resumes with the same reading rhythm.",
  "",
  "### Detailed steps",
  "",
  "1. Capture the result before reviewing the details.",
  "",
  "   - Keep the response tied to its accepted request.",
  "   - Preserve the source for later comparison.",
  "",
  "   Resume the explanation after the nested evidence.",
  "2. Verify the projected view and its public actions.",
  "",
  "> 1. [x] Preserve the accepted result",
  ">",
  ">    Keep this follow-up paragraph in the same recorded task.",
  "> 2. [ ] Review the next projection",
  "",
  "### Source references",
  "",
  "The implementation lives in apps/web/src/features/chat/message/ui/markdown/markdownComponents.tsx:12:3. Continue reading the surrounding explanation without a separate block.",
  "",
  "在 src/main.ts:12:3 中检查边界，保持文件引用与中文正文的阅读节奏。",
  "",
  "Review README.md:15:2 before publishing, then inspect apps/web/package.json. The sentence keeps its final punctuation.",
  "",
  "### Reference material",
  "",
  "Read [the protocol guide][protocol] before continuing. The [implementation notes][] explain how the accepted result reaches its consumer.",
  "",
  "The recorded release is available at https://example.test/runtime/releases. Keep its address with the surrounding explanation.",
  "",
  'Read the [**Review** `guide`](https://example.test/runtime/review "Accepted evidence and follow-up") before publishing the next result.',
  "",
  "An [unavailable source](javascript:alert%281%29) remains part of the recorded explanation.",
  "",
  '[protocol]: https://example.test/runtime/protocol "Accepted protocol contract"',
  "[implementation notes]: https://example.test/runtime/implementation",
].join("\n");

const REFERENCE_NOTES_MARKDOWN = [
  "## A clear answer with supporting notes",
  "",
  "The consumer keeps the accepted observation next to the decision it supports.[^边界] The explanation continues along the same reading edge, without turning the reference into a separate object.",
  "",
  "### What to verify",
  "",
  "- Keep the original request with its accepted result.",
  "- Read the surrounding context before advancing the next state.",
  "- Preserve the public contract when the view becomes narrower.",
  "",
  "中文说明也应保留完整的引用关系，第二次引用返回到它自己所在的位置。[^边界]",
  "",
  'The [protocol guide](https://example.test/runtime-guide "Read the recorded contract") supplies the public vocabulary. The answer then resumes in ordinary prose.',
  "",
  "[^边界]: The Runtime owns the durable fact. The client reads an accepted projection.",
  "",
  "    Supporting context stays in a second paragraph, with enough room to distinguish it from the main answer.",
].join("\n");

export const CALLOUT_MARKDOWN = [
  "## Notes that belong to the answer",
  "",
  "The review should read as one answer, with supporting notes kept close to the evidence.",
  "",
  "> [!NOTE]",
  "> Keep the accepted result with the observation that explains it.",
  ">",
  "> A second paragraph adds context without starting a separate card.",
  "",
  "The explanation resumes along the same reading edge, so the next decision stays easy to follow.",
  "",
  "> [!WARNING]",
  "> 更新之前，请先检查 `runtime.Commit` 的调用边界，并保留本次审阅的上下文。",
  ">",
  "> - Confirm the recorded result.",
  "> - Review the consumer before continuing.",
  "",
  "> A plain quotation remains an ordinary part of the review.",
  "",
  "The answer ends with its conclusion, rather than another layer of interface chrome.",
].join("\n");

const CALLOUT_RESPONSE = message("agentMessage", "item_callout_response", CALLOUT_MARKDOWN);

const MATH_RESPONSE = message(
  "agentMessage",
  "item_math_response",
  [
    "## A readable derivation",
    "",
    "计算结果应当与正文沿用同一条阅读边界。Inline math such as $E = mc^2$ stays in its sentence.",
    "",
    "$$",
    "E = mc^2",
    "$$",
    "",
    "### A wide objective",
    "",
    "$$",
    "\\mathcal{L}(\\theta) = \\sum_{i=1}^{N} \\left[-y_i \\log p_{\\theta}(x_i) - (1-y_i)\\log(1-p_{\\theta}(x_i))\\right] + \\lambda\\sum_{j=1}^{M}\\theta_j^2 + \\mu\\sum_{k=1}^{K}\\left|\\theta_{k+1}-\\theta_k\\right| + \\gamma\\operatorname{Var}_{i=1}^{N}\\left[p_{\\theta}(x_i)\\right] \\tag{1}",
    "$$",
    "",
    "The equation remains one mathematical expression. Its first term and final tag both need to stay reachable.",
    "",
    "### A wide matrix",
    "",
    "$$",
    "A = \\begin{bmatrix}a_{1,1} & a_{1,2} & a_{1,3} & a_{1,4} & a_{1,5} & a_{1,6} & a_{1,7} & a_{1,8} & a_{1,9} & a_{1,10}\\\\a_{2,1} & a_{2,2} & a_{2,3} & a_{2,4} & a_{2,5} & a_{2,6} & a_{2,7} & a_{2,8} & a_{2,9} & a_{2,10}\\end{bmatrix}",
    "$$",
    "",
    "### A tall expression",
    "",
    "$$",
    "F(x) = \\frac{\\displaystyle\\sum_{i=1}^{N}\\frac{x_i^2}{1+x_i}}{\\sqrt{\\frac{1+\\sum_{j=1}^{M}x_j^2}{1+\\left|\\sum_{j=1}^{M}x_j\\right|}}}",
    "$$",
    "",
    "After the derivation, the explanation resumes in ordinary prose without changing the reading measure.",
  ].join("\n"),
);

const NARRATIVE_TURN_1 = message(
  "userMessage",
  "item_n_ask1",
  "Pull the retry logic out of checkout into its own hook, run the tests, then show me the tradeoffs.",
);

const NARRATIVE_REASONING: Item = {
  type: "reasoning",
  id: "item_n_reasoning",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:01:00.000Z",
  text: "The retry loop is inlined in handleSubmit with a hardcoded ceiling and no backoff. Extracting it has two hazards: the idempotency key has to survive the whole retry cycle, and the component may unmount mid-flight.",
};

function narrativeTool(
  id: string,
  name: string,
  args: Record<string, unknown>,
  result?: unknown,
): Item {
  return {
    type: "toolCall",
    id,
    runId: ROOT_RUN_ID,
    status: "completed",
    startedAt: "2026-07-31T08:01:04.000Z",
    durationMillis: 120,
    finishedAt: "2026-07-31T08:01:04.120Z",
    safetyClass: safetyClassOf(name),
    tool: {
      name,
      category: toolCategoryForName(name),
      arguments: args,
      ...(result === undefined ? {} : { result }),
    },
  };
}

const NARRATIVE_ANSWER_1 = message(
  "agentMessage",
  "item_n_answer1",
  [
    "## The extracted hook",
    "",
    "The key is pinning the idempotency key to a `useRef` so the whole retry cycle shares one — otherwise every attempt reads as a new order at the gateway.",
    "",
    "- **Exponential backoff**: 400ms → 800ms → 1600ms, three attempts;",
    "- **Cancellable**: aborts on unmount and stops writing state;",
    "- **Observable**: every failure reports a `payment.retry` event.",
    "",
    "## Strategy comparison",
    "",
    "| Strategy | P95 success | Double-charge risk | Cost |",
    "| --- | --- | --- | --- |",
    "| Fixed interval | 91.2% | High | Low |",
    "| Exponential backoff | 96.8% | Medium | Low |",
    "| Backoff + jitter + key | 98.4% | Low | Medium |",
    "| Server-side replay queue | 99.1% | Low | High |",
    "",
    "Sampled from the payment gateway over 2026-07, n = 41,208.",
  ].join("\n"),
);

const NARRATIVE_TURN_2 = message(
  "userMessage",
  "item_n_ask2",
  "There are still a few double charges in production. Check the errors and give me two fixes.",
);

const NARRATIVE_ANSWER_2 = message(
  "agentMessage",
  "item_n_answer2",
  [
    "## Two ways to fix it",
    "",
    "Both remove the double charge; they differ in which layer owns the key's lifetime.",
    "",
    "1. **Key follows the order** — mint it once at checkout and persist it, so a refresh reuses it. Two files, no backend work.",
    "2. **Server issues the key** — the checkout endpoint returns an intent id and the client only forwards it. Five files, one backend day.",
  ].join("\n"),
);

const NARRATIVE_TURN_3 = message(
  "userMessage",
  "item_n_ask3",
  "Go with the first one, and add a regression test for the refresh case.",
);

const CANCELED_QUESTION: Item = {
  type: "question",
  id: "item_canceled_question",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:00:08.000Z",
  question: {
    fields: [{ type: "text", header: "Scope", prompt: "Should the review cover the CLI too?" }],
  },
};

const NARRATIVE_ANSWERED_QUESTION: Item = {
  type: "question",
  id: "item_n_question",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:01:30.000Z",
  question: {
    fields: [
      {
        type: "choice",
        header: "Key lifetime",
        prompt: "Where should the idempotency key be minted?",
        options: [
          { label: "At checkout", description: "One key per order, persisted with it." },
          { label: "At send time", description: "A new key on every attempt." },
        ],
      },
      {
        type: "text",
        header: "Rollout",
        prompt: "Anything to hold back?",
      },
    ],
    answers: [["At checkout"], ["Ship behind the existing payments flag."]],
  },
};

const NARRATIVE_COMPACTION: Item = {
  type: "compaction",
  id: "item_n_compaction",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:02:00.000Z",
  droppedMessages: 34,
  summary: "Earlier tool output folded into a summary.",
};

const BASE: RuntimeAgentSessionSnapshot = {
  runs: [],
  items: [],
  pendingInterruptSets: [],
};

const FANOUT_OUTCOMES: ReadonlyArray<{ id: string; summary: string; outcome: RunRef["outcome"] }> =
  [
    {
      id: "run_fan_ok",
      summary: "Check the module graph",
      outcome: { type: "completed" },
    },
    {
      id: "run_fan_err",
      summary: "Rebuild the provider fixtures",
      outcome: {
        type: "failed",
        error: { type: "tool_failed", detail: "The fixture generator exited 1." },
      },
    },
    {
      id: "run_fan_canceled",
      summary: "Survey the CLI surface",
      outcome: { type: "canceled", detail: "Stopped once the answer was already known." },
    },
    {
      id: "run_fan_trace",
      summary: "Trace every call path",
      outcome: { type: "canceled", detail: "The user canceled this delegation." },
    },
  ];

const FANOUT_RUNS: RunRef[] = FANOUT_OUTCOMES.map((fan, index) => ({
  id: fan.id,
  sessionId: SESSION_ID,
  status: "finished",
  createdAt: "2026-07-31T08:00:07.000Z",
  finishedAt: `2026-07-31T08:00:${String(8 + index).padStart(2, "0")}.000Z`,
  outcome: fan.outcome,
  metrics: { steps: 2 + index, activeDurationMillis: 4_000 },
  provider: RUN_PROVIDER,
  model: RUN_MODEL,
  parentRunId: ROOT_RUN_ID,
  rootRunId: ROOT_RUN_ID,
  spawnedByItemId: "item_fanout",
}));

const FANOUT_DELEGATE: Item = {
  type: "toolCall",
  id: "item_fanout",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: "2026-07-31T08:00:07.000Z",
  durationMillis: 5_000,
  finishedAt: "2026-07-31T08:00:12.000Z",
  safetyClass: "safe",
  tool: {
    name: "delegate_task",
    category: "task",
    arguments: {
      summary: "Audit persistence boundaries across Runtime, Framework and delegated agents",
      instructions: "…",
    },
  },
};

export const RUNTIME_AGENT_SESSION_SNAPSHOTS: Readonly<
  Record<VisualAgentState, RuntimeAgentSessionSnapshot>
> = {
  empty: BASE,
  "runtime-down": BASE,
  idle: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  "answer-opening": {
    runs: [run("running")],
    items: [PROMPT],
    pendingInterruptSets: [],
    plan: {
      revision: 3,
      steps: [
        { id: "step_boundary", text: "Verify boundary ownership", status: "done" },
        { id: "step_visual", text: "Review visual evidence", status: "active" },
        { id: "step_gates", text: "Run quality gates", status: "pending" },
      ],
    },
  },
  running: {
    runs: [run("running")],
    items: [PROMPT],
    pendingInterruptSets: [],
    plan: {
      revision: 3,
      steps: [
        { id: "step_boundary", text: "Verify boundary ownership", status: "done" },
        { id: "step_visual", text: "Review visual evidence", status: "active" },
        { id: "step_gates", text: "Run quality gates", status: "pending" },
      ],
    },
  },
  steer: {
    runs: [run("running")],
    items: [PROMPT],
    pendingInterruptSets: [],
    plan: {
      revision: 3,
      steps: [
        { id: "step_boundary", text: "Verify boundary ownership", status: "done" },
        { id: "step_visual", text: "Review visual evidence", status: "active" },
        { id: "step_gates", text: "Run quality gates", status: "pending" },
      ],
    },
  },
  waiting: {
    runs: [run("waiting")],
    items: [PROMPT, COMMENTARY_RESPONSE, PENDING_APPROVAL_TOOL],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:09.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_approval",
            runId: ROOT_RUN_ID,
            payload: {
              tool: {
                name: "shell",
                arguments: {
                  description: "Run the race detector",
                  command: "go test -race ./...",
                },
              },
              reason: "Run the race detector across the workspace before committing.",
              rememberable: true,
            },
          },
        ],
      },
    ],
  },
  "question-multi": {
    runs: [run("waiting")],
    items: [PROMPT, COMMENTARY_RESPONSE],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:09.000Z",
        interrupts: [
          {
            type: "question",
            itemId: "item_question_multi",
            runId: ROOT_RUN_ID,
            payload: {
              question: {
                fields: [
                  {
                    type: "choice",
                    multiple: true,
                    allowCustom: true,
                    header: "Gates",
                    prompt: "Which gates should run before the release?",
                    options: [
                      {
                        label: "Race detector",
                        description: "Exercise concurrency and cancellation paths.",
                        preview: "go test -race ./...",
                      },
                      {
                        label: "Visual suite",
                        description: "Verify light, dark, long-content, and HITL states.",
                        preview: "npm run test:visual",
                      },
                      {
                        label: "Conformance",
                        description: "Replay the canonical wire samples against the schema.",
                        preview: "npm run check:api-consumers",
                      },
                      {
                        label:
                          "Run all release checks for the selected workspace and include concurrency diagnostics",
                        description: "",
                      },
                      {
                        label:
                          "Run all release checks for the selected workspace and include accessibility diagnostics",
                        description: "Keep the full option visible before choosing a release gate.",
                      },
                    ],
                  },
                ],
              },
            },
          },
        ],
      },
    ],
  },

  question: {
    runs: [run("waiting")],
    items: [PROMPT, COMMENTARY_RESPONSE],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:09.000Z",
        interrupts: [
          {
            type: "question",
            itemId: "item_question",
            runId: ROOT_RUN_ID,
            payload: {
              question: {
                fields: [
                  {
                    type: "choice",
                    header: "Gate",
                    prompt: "Which gate should run next?",
                    options: [
                      {
                        label: "Race detector",
                        description: "Exercise concurrency and cancellation paths.",
                        preview: "go test -race ./...",
                      },
                      {
                        label: "Visual suite",
                        description: "Verify light, dark, long-content, and HITL states.",
                        preview: "npm run test:visual",
                      },
                    ],
                  },
                  {
                    type: "text",
                    header: "Context",
                    prompt: "What should this gate protect?",
                  },
                ],
              },
            },
          },
        ],
      },
    ],
  },
  terminal: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  canceled: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "canceled", detail: "Stopped after the requested review." },
      }),
    ],
    items: [PROMPT, COMMENTARY_RESPONSE, CANCELED_QUESTION],
    pendingInterruptSets: [],
  },
  error: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: {
          type: "failed",
          error: {
            type: "provider_rejected",
            detail: "the requested model is unavailable",
          },
        },
      }),
    ],
    items: [PROMPT],
    pendingInterruptSets: [],
  },
  "error-retryable": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "failed", error: { type: "provider_error" } },
      }),
    ],
    items: [PROMPT],
    pendingInterruptSets: [],
  },
  recovery: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: {
          type: "lost",
          error: {
            type: "run_lost",
            detail: "The Runtime restarted before this Run reached a terminal event.",
          },
        },
      }),
    ],
    items: [PROMPT],
    pendingInterruptSets: [],
  },
  "cwd-missing": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  delegated: {
    runs: [
      run("running"),
      {
        id: "run_child",
        sessionId: SESSION_ID,
        status: "waiting",
        createdAt: "2026-07-31T08:00:03.000Z",
        metrics: { steps: 2, activeDurationMillis: 7_000 },
        provider: RUN_PROVIDER,
        model: RUN_MODEL,
        parentRunId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: "item_delegate",
      },
      ...FANOUT_RUNS,
      {
        id: "run_nested",
        sessionId: SESSION_ID,
        status: "running",
        activeSegmentId: "seg_nested",
        createdAt: "2026-07-31T08:00:05.000Z",
        metrics: { steps: 1, activeDurationMillis: 3_000 },
        provider: RUN_PROVIDER,
        model: RUN_MODEL,
        parentRunId: "run_child",
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: "item_nested_delegate",
      },
    ],
    items: [
      PROMPT,
      COMMENTARY_RESPONSE,
      {
        type: "toolCall",
        id: "item_delegate",
        runId: ROOT_RUN_ID,
        status: "completed",
        startedAt: "2026-07-31T08:00:02.000Z",
        durationMillis: 940,
        finishedAt: "2026-07-31T08:00:02.940Z",
        safetyClass: "safe",
        tool: {
          name: "delegate_task",
          category: "task",
          arguments: { summary: "Audit Agent Framework ownership", instructions: "…" },
        },
      },
      message(
        "agentMessage",
        "item_child_response",
        `The framework remains generic. I found no application persistence type in its production graph.\n\n${LONG_RESPONSE_MARKDOWN}`,
        "run_child",
        "commentary",
      ),
      {
        type: "toolCall",
        id: "item_nested_delegate",
        runId: "run_child",
        status: "completed",
        startedAt: "2026-07-31T08:00:04.000Z",
        durationMillis: 610,
        finishedAt: "2026-07-31T08:00:04.610Z",
        safetyClass: "safe",
        tool: {
          name: "delegate_task",
          category: "task",
          arguments: { summary: "Verify package dependencies", instructions: "…" },
        },
      },
      message(
        "agentMessage",
        "item_nested_response",
        "Package graph verification is still running.",
        "run_nested",
        "commentary",
      ),
      FANOUT_DELEGATE,
      message(
        "userMessage",
        "item_fan_ok_input",
        "Inspect the complete module graph and explain where persistence belongs.\n\nKeep the Runtime boundary explicit, show the evidence from source, and compare the public application contracts.\n\nInclude nested findings, source examples, tables, and mixed-language observations so the review remains readable in a narrow workspace.",
        "run_fan_ok",
      ),
      message("agentMessage", "item_fan_ok_answer", LONG_RESPONSE_MARKDOWN, "run_fan_ok"),
      {
        type: "toolCall",
        id: "item_nested_running",
        runId: "run_nested",
        status: "running",
        startedAt: "2026-07-31T08:00:06.000Z",
        safetyClass: "safe",
        tool: {
          name: "grep",
          arguments: { pattern: "grid-cols-subgrid", path: "runtime" },
        },
      },
    ],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:06.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_child_approval",
            runId: "run_child",
            payload: {
              tool: { name: "shell", arguments: { command: "go list -deps ./..." } },
              reason: "Inspect the complete dependency graph.",
            },
          },
        ],
      },
    ],
  },
  "long-content": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, LONG_RESPONSE],
    pendingInterruptSets: [],
  },
  "prose-blocks": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, message("agentMessage", "item_reading_blocks", READING_BLOCKS_MARKDOWN)],
    pendingInterruptSets: [],
  },
  "code-reading": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, message("agentMessage", "item_code_reading", CODE_READING_MARKDOWN)],
    pendingInterruptSets: [],
  },
  "table-reading": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, message("agentMessage", "item_table_reading", TABLE_READING_MARKDOWN)],
    pendingInterruptSets: [],
  },
  "reference-notes": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [
      PROMPT,
      message("agentMessage", "item_notes_1", REFERENCE_NOTES_MARKDOWN),
      message(
        "agentMessage",
        "item_notes_2",
        REFERENCE_NOTES_MARKDOWN.replace("A clear answer", "Another answer"),
      ),
    ],
    pendingInterruptSets: [],
  },
  callouts: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, CALLOUT_RESPONSE],
    pendingInterruptSets: [],
  },
  "math-content": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, MATH_RESPONSE],
    pendingInterruptSets: [],
  },
  narrative: {
    runs: [run("waiting")],
    items: [
      NARRATIVE_TURN_1,
      NARRATIVE_REASONING,
      narrativeTool(
        "item_n_read",
        "read",
        { path: "src/checkout/checkout.tsx" },
        { content: "export function Checkout() {\n  …\n}\n", total_lines: 212 },
      ),
      narrativeTool(
        "item_n_read_2",
        "read",
        {
          path: "/Users/visual/scope/apps/web/src/features/chat/tools/ui/ToolGroup.tsx",
        },
        { content: "export function ToolGroup() {\n  …\n}\n", total_lines: 148 },
      ),
      narrativeTool(
        "item_n_read_3",
        "read",
        { path: "src/checkout/api/pay.ts" },
        { content: "export async function pay() {\n  …\n}\n", total_lines: 96 },
      ),
      narrativeTool(
        "item_n_grep",
        "grep",
        { pattern: "retry|backoff", path: "src" },
        {
          hits: [
            { path: "src/checkout/api/pay.ts", lineNumber: 41, snippet: "  const retry = () => {" },
            { path: "src/checkout/api/pay.ts", lineNumber: 58, snippet: "    backoff(attempt);" },
            {
              path: "src/checkout/hooks/useRetryPayment.ts",
              lineNumber: 12,
              snippet: "export function useRetryPayment() {",
            },
          ],
        },
      ),
      narrativeTool(
        "item_n_patch",
        "apply_patch",
        { patch: "*** Begin Patch\n*** Add File: useRetryPayment.ts\n…\n*** End Patch" },
        {
          changes: [
            {
              path: "src/checkout/hooks/useRetryPayment.ts",
              status: "added",
              added: 0,
              removed: 0,
            },
          ],
        },
      ),
      NARRATIVE_ANSWER_1,
      NARRATIVE_TURN_2,
      NARRATIVE_ANSWERED_QUESTION,
      NARRATIVE_COMPACTION,
      narrativeTool(
        "item_n_search",
        "web_search",
        { query: "stripe idempotency key retry" },
        {
          results: [
            {
              title: "Idempotent requests",
              url: "https://docs.stripe.com/api/idempotent_requests",
              snippet: "Keys expire after 24 hours; a replay returns the original response.",
            },
            {
              title: "Handling retries safely",
              url: "https://stripe.com/blog/idempotency",
              snippet: "Mint the key at the point the user commits, not at send time.",
            },
          ],
        },
      ),
      NARRATIVE_ANSWER_2,
      NARRATIVE_TURN_3,
    ],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:02:20.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_n_approval",
            runId: ROOT_RUN_ID,
            payload: {
              tool: {
                name: "shell",
                arguments: { command: "rm -rf node_modules .next && pnpm install" },
              },
              reason: "rm -rf deletes uncommitted build output and cannot be undone.",
              rememberable: true,
            },
          },
        ],
      },
    ],
  },
  "tool-tail": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:05.000Z",
        outcome: { type: "completed" },
        metrics: { steps: 5, activeDurationMillis: 5_000 },
      }),
    ],
    items: [
      PROMPT,
      PROPOSE_SKILL_CALL,
      SKILL_RESOURCE_CALL,
      READ_TOOL_RESULT_CALL,
      STOP_SHELL_CALL,
      ASK_USER_CALL,
      RESPONSE,
    ],
    pendingInterruptSets: [],
  },

  "tool-agentic": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:06.000Z",
        outcome: { type: "completed" },
        metrics: { steps: 6, activeDurationMillis: 6_000 },
      }),
    ],
    items: [
      PROMPT,
      LIST_SKILLS_CALL,
      LOAD_SKILL_CALL,
      ENTER_PLAN_CALL,
      GET_GOAL_CALL,
      REPORT_GOAL_CALL,
      SCHEDULES_CALL,
      READ_SHELL_CALL,
      LSP_CALL,
      WORKSPACE_INSPECTION_CALL,
      RESPONSE,
    ],
    pendingInterruptSets: [],
  },

  "tool-remote": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:05.000Z",
        outcome: { type: "completed" },
        metrics: { steps: 3, activeDurationMillis: 5_000 },
      }),
    ],
    items: [PROMPT, WEB_SEARCH_CALL, WEB_FETCH_CALL, HTTP_CALL, SCHEDULES_CALL, RESPONSE],
    pendingInterruptSets: [],
  },

  "tool-search": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:04.000Z",
        outcome: { type: "completed" },
        metrics: { steps: 4, activeDurationMillis: 4_000 },
      }),
    ],
    items: [PROMPT, GLOB_CALL, MEMORY_CALL, TOOL_SEARCH_CALL, RESPONSE],
    pendingInterruptSets: [],
  },

  "tool-shells": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
        metrics: { steps: 6, activeDurationMillis: 12_000 },
      }),
    ],
    items: [
      PROMPT,
      SHELL_READ,
      SHELL_COMMAND,
      SHELL_PATCH,
      PATCH_NEW_FILE,
      SHELL_FAILED,
      SHELL_DENIED,
      RESPONSE,
    ],
    pendingInterruptSets: [],
  },

  waves: {
    runs: [run("running")],
    items: [
      PROMPT,
      WAVE_REASONING_ONE,
      WAVE_READ,
      WAVE_GREP,
      WAVE_ANSWER_ONE,
      WAVE_REASONING_TWO,
      WAVE_PATCH,
      WAVE_ANSWER_TWO,
    ],
    pendingInterruptSets: [],
  },
};

export function delegatedApprovalSnapshot(): RuntimeAgentSessionSnapshot {
  const snapshot = structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.delegated);
  snapshot.runs = snapshot.runs.map((value): RunRef => {
    const parked = { ...value };
    delete parked.activeSegmentId;
    if (value.id === ROOT_RUN_ID) return { ...parked, status: "waiting" };
    if (value.id === "run_nested")
      return {
        ...parked,
        status: "finished",
        finishedAt: "2026-07-31T08:00:07.000Z",
        outcome: { type: "completed" },
      };
    return value;
  });
  snapshot.items = snapshot.items.map((item) =>
    item.type === "toolCall" && item.id === "item_nested_running"
      ? {
          ...item,
          status: "completed",
          finishedAt: "2026-07-31T08:00:07.000Z",
          durationMillis: 1_000,
          tool: { ...item.tool, result: { hits: [] } },
        }
      : item,
  );
  return snapshot;
}

export function projectRuntimeAgentSnapshot(
  snapshot: RuntimeAgentSessionSnapshot,
): AgentSessionSnapshot {
  return {
    ...snapshot,
    runs: snapshot.runs.map(runtimeRunFact),
    items: snapshot.items.map(runtimeItem),
    pendingInterruptSets: snapshot.pendingInterruptSets.map(runtimePendingInterruptSet),
  };
}

export const RUNTIME_AGENT_SESSION_TAIL_EVENTS: Readonly<Record<VisualAgentState, TailFrame[]>> = {
  empty: [],
  "runtime-down": [],
  idle: [],
  running: [
    tail(1, { type: "item.started", item: RUNNING_REASONING }),
    tail(2, { type: "item.completed", item: RUNNING_SET_PLAN }),
    tail(3, { type: "item.completed", item: RUNNING_READ }),
    tail(4, { type: "item.started", item: RUNNING_TOOL }),
    tail(5, { type: "item.started", item: RUNNING_RESPONSE }),
    tail(6, {
      type: "segment.progress",
      progress: { contextTokens: VISUAL_CONTEXT_TOKENS },
    }),
  ],
  "answer-opening": [
    tail(1, { type: "item.started", item: RUNNING_REASONING }),
    tail(2, { type: "item.completed", item: RUNNING_SET_PLAN }),
    tail(3, { type: "item.completed", item: RUNNING_READ }),
    tail(4, { type: "item.started", item: RUNNING_TOOL }),
    tail(5, { type: "item.started", item: OPENING_RESPONSE }),
    tail(6, {
      type: "segment.progress",
      progress: { contextTokens: VISUAL_CONTEXT_TOKENS },
    }),
  ],
  steer: [
    tail(1, { type: "item.started", item: RUNNING_REASONING }),
    tail(3, { type: "item.completed", item: RUNNING_READ }),
    tail(4, { type: "item.started", item: RUNNING_TOOL }),
    tail(5, { type: "item.started", item: RUNNING_RESPONSE }),
    tail(6, {
      type: "segment.progress",
      progress: { contextTokens: VISUAL_CONTEXT_TOKENS },
    }),
  ],
  waiting: [],
  question: [],
  terminal: [],
  canceled: [],
  error: [],
  "error-retryable": [],
  "cwd-missing": [],
  recovery: [],
  delegated: [],
  "long-content": [],
  "prose-blocks": [],
  "code-reading": [],
  "table-reading": [],
  "reference-notes": [],
  "math-content": [],
  callouts: [],
  narrative: [],
  "tool-shells": [],
  "tool-search": [],
  "tool-remote": [],
  "tool-agentic": [],
  "tool-tail": [],
  "question-multi": [],
  waves: [
    tail(1, { type: "item.started", item: WAVE_LIVE_REASONING }),
    tail(2, { type: "item.started", item: WAVE_LIVE_TOOL }),
  ],
};

export const AGENT_SESSION_TAIL_EVENTS: Readonly<Record<VisualAgentState, AgentEventEnvelope[]>> =
  Object.fromEntries(
    Object.entries(RUNTIME_AGENT_SESSION_TAIL_EVENTS).map(([state, frames]) => [
      state,
      frames.map((frame) => tailEvent(frame.index, frame.event)),
    ]),
  ) as Record<VisualAgentState, AgentEventEnvelope[]>;

export const VISUAL_GOALS: Partial<Record<VisualAgentState, GoalReadModel>> = {
  running: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux without loosening any gate or skipping a test",
    status: "active",
    used: { runs: 7, costUsd: 4.5, steps: 31 },
    createdAt: "2026-08-12T08:00:00Z",
  },
  terminal: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux",
    status: "paused",
    used: { runs: 12, costUsd: 5, steps: 58 },
    createdAt: "2026-08-12T08:00:00Z",
  },
  canceled: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux",
    status: "blocked",
    used: { runs: 4, costUsd: 1.2, steps: 18 },
    createdAt: "2026-08-12T08:00:00Z",
  },
  steer: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux",
    status: "completing",
    used: { runs: 19, costUsd: 4.9, steps: 96 },
    createdAt: "2026-08-12T08:00:00Z",
  },
};

export const VISUAL_SESSION_ID = SESSION_ID;

export const VISUAL_ROOT_RUN_ID = ROOT_RUN_ID;
