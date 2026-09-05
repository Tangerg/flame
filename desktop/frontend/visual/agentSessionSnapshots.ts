import type { Item, PendingInterruptSet, RunProtocolProfile, RunRef, StreamEvent } from "@/rpc";
import type { AgentEventEnvelope } from "@/plugins/sdk";
import {
  runtimeAgentEvent,
  runtimeItem,
  runtimePendingInterruptSet,
  runtimeRunFact,
} from "@/plugins/builtin/agent/adapters/runtimeAgentFacts";
import type { AgentSessionSnapshot } from "@/plugins/builtin/agent/application/ports/runtimeGateway";
import type { GoalReadModel } from "@/plugins/builtin/chat/goal/application/goalReadModel";
import { VISUAL_CONTEXT_TOKENS } from "./agentFixtureFacts";

export const VISUAL_AGENT_STATES = [
  "empty",
  "idle",
  "running",
  "answer-opening",
  "steer",
  "waiting",
  "question",
  "terminal",
  "canceled",
  "error",
  "recovery",
  "delegated",
  "long-content",
  "narrative",
  "tool-shells",
  "tool-search",
  "tool-remote",
  "tool-agentic",
  "waves",
] as const;

export type VisualAgentState = (typeof VISUAL_AGENT_STATES)[number];

const SESSION_ID = "ses_visual";
const ROOT_RUN_ID = "run_root";
const CREATED_AT = "2026-07-31T08:00:00.000Z";
const RUN_PROVIDER = "openai";
const RUN_MODEL = "gpt-5.6-sol";
// Cumulative accounting remains a Run fact. It deliberately does NOT drive the
// composer's context gauge: window occupancy is the latest Runtime contextTokens
// reading, supplied independently in the live event tail below.
const METRICS = {
  steps: 4,
  activeDurationMillis: 12_000,
  usage: { inputTokens: 96_000, outputTokens: 3_100, cacheReadTokens: 64_000 },
};
const PROFILE: RunProtocolProfile = {
  interruptTypes: ["approval"],
  requiredFeatures: ["subagents"],
};

type RuntimeAgentSessionSnapshot = Omit<
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
  delegate_task: "safe",
  edit: "write",
  write: "write",
  shell: "exec",
};

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

// The answer's item as `item.started` actually delivers it: opened, with no content
// yet. Every turn passes through this frame, and no fixture held it — the running
// state's response arrives with a sentence already in it, which `item.started` never
// carries. Freezing the frame is what makes "the thinking is still open here"
// something a screenshot can be wrong about.
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
  text: "The framework must expose execution capability without knowing the application’s persistence records.",
};

// Writing the plan, which the banner above the transcript already holds. The fixture
// carries it so the duplication is reproducible: this call must leave no row behind.
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

// A settled read ahead of the running one, so the two fold into a tool GROUP:
// a disclosure nested inside a disclosure, auto-open because a child is still
// working. That is the shape a real working turn spends most of its time in, and
// until now no fixture rendered it — which is how a nested row overflowing its
// parent's rounded corner shipped twice.
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

// A waiting approval keeps the original ToolCall Item as well as the interrupt.
// Production restores both facts from the Runtime snapshot; the transcript planner
// owns turning that pair into one actionable request surface.
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

// One turn holding a read, a command, a patch, a failure and a refusal — every lifecycle a
// tool row has to survive on the SAME narrative line. That sameness is the thing under test:
// the differentiation lives in the verb, the diffstat and the inline lifecycle text, so a
// golden that only held successful reads would not notice a failure growing card chrome.
// Four searching tools, none of which any fixture had ever called. Thirty tool names carry a
// preview and six were exercised, so twenty-four of these panels — their placeholders, their
// overflow rules, their inks — had never been rendered, let alone photographed or audited.
// The result strings are the shapes each projection parses, not prose about them.
function searchTool(
  id: string,
  name: string,
  args: Record<string, unknown>,
  result: unknown,
): Item {
  return {
    type: "toolCall",
    safetyClass: "safe",
    id,
    runId: ROOT_RUN_ID,
    status: "completed",
    startedAt: CREATED_AT,
    durationMillis: 120,
    finishedAt: "2026-07-31T08:00:00.120Z",
    tool: { name, arguments: args, result },
  };
}

const GLOB_CALL = searchTool(
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

const MEMORY_CALL = searchTool(
  "item_search_memory",
  "search_memory",
  { query: "compaction cutpoint" },
  [
    "1. The compaction cutpoint is chosen by the Runtime, never by the client.",
    "2. A steer arriving during compaction is queued, not dropped.",
  ].join("\n"),
);

const CONVERSATIONS_CALL = searchTool(
  "item_search_conversations",
  "search_conversations",
  { query: "atomicity" },
  [
    "1. [user · 2026-07-24] Where does the transaction boundary sit?",
    "2. [assistant · 2026-07-24] Around the store call, so a failed flush rolls the write back.",
  ].join("\n"),
);

const TOOL_SEARCH_CALL = searchTool(
  "item_search_tools",
  "search_tools",
  { query: "schedule" },
  [
    "Not loaded:",
    "  [flame] create_schedule, list_schedules, delete_schedule",
    "  [mcp:github] create_issue",
  ].join("\n"),
);

// A second batch of previews no fixture had called: the ones that answer in JSON rather than
// prose. Each result is the exact shape its projection reads, so a preview that stops parsing
// fails here instead of degrading to a blank panel in front of someone.
const WEB_SEARCH_CALL = searchTool(
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

const WEB_FETCH_CALL = searchTool(
  "item_remote_web_fetch",
  "web_fetch",
  { url: "https://www.sqlite.org/wal.html" },
  JSON.stringify({
    format: "markdown",
    content:
      "# Write-Ahead Logging\n\nA checkpoint moves frames from the WAL back into the database.",
  }),
);

const HTTP_CALL = searchTool(
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

const SCHEDULES_CALL = searchTool(
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
      // Disabled, because the row says so in a way only a disabled one can show.
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

// A third batch: the agent's own machinery — skills it can load, a plan it enters and leaves,
// a goal it reports on, a background shell it reads back, and the language server. None of
// these panels had ever been rendered either.
const LIST_SKILLS_CALL = searchTool(
  "item_agentic_list_skills",
  "list_skills",
  {},
  [
    "<skill><name>review-diff</name><description>Read a change the way a reviewer does, worst risk first.</description></skill>",
    "<skill><name>trace-flaky-test</name><description>Find the shared state behind a test that passes alone.</description></skill>",
  ].join("\n"),
);

// `LoadSkillResult` is the Skill's own instructions, not the catalogue envelope `list_skills`
// answers in — CONTENT_RENDERING states both, and the preview prints the text because the text
// is what arrives. The first draft of this fixture reused the envelope and made the panel look
// broken; the panel was right and the fixture was lying to it.
const LOAD_SKILL_CALL = searchTool(
  "item_agentic_load_skill",
  "load_skill",
  { name: "review-diff" },
  [
    "Start from the riskiest hunk, not the first one.",
    "Name the invariant the change could break, then look for the test that would catch it.",
    "A diff with no test change is a claim that nothing observable changed. Check it.",
  ].join("\n"),
);

const ENTER_PLAN_CALL = searchTool(
  "item_agentic_enter_plan",
  "enter_plan_mode",
  {},
  "Planning only from here: no edits until the plan is accepted.",
);

// Three families that deliberately render NO row. Plan, Goal and Schedule each have a
// dedicated surface — the plan bar, the goal bar, the Schedules pane — and `BlockRenderer`
// drops the transcript row of any tool registered against one, because a row would be a second
// telling of what that surface already holds. Kept here so the rule is photographed rather than
// assumed: this state issues four such calls and the golden shows none of them.
const GET_GOAL_CALL = searchTool(
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

const REPORT_GOAL_CALL = searchTool(
  "item_agentic_report_goal",
  "report_goal_outcome",
  { outcome: "achieved" },
  "Goal achieved: the boundary holds.",
);

const READ_SHELL_CALL = searchTool(
  "item_agentic_read_shell",
  "read_shell_output",
  { shell_id: "sh_01" },
  [
    "waiting for the build to settle",
    "ok  \tgithub.com/Tangerg/flame/runtime/internal/run\t2.104s",
  ].join("\n"),
);

const LSP_CALL = searchTool(
  "item_agentic_lsp",
  "lsp",
  // `character` is required for a position operation; without it the row titles itself
  // `store.go:214:?`, which is the app being honest about an argument the caller left out.
  { operation: "references", path: "runtime/internal/session/store.go", line: 214, character: 6 },
  [
    "runtime/internal/session/store.go:214:6",
    "runtime/internal/session/atomicity.go:88:14",
    "runtime/internal/run/segment.go:41:9",
  ].join("\n"),
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
  tool: { name: "read", arguments: { path: "runtime/internal/session/store.go" } },
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
  // A real test run: coloured, longer than the panel shows at rest, and non-zero.
  // Every one of those was invisible here — the fixture's command produced no output
  // at all, so the panel that holds it appeared in no screenshot.
  tool: {
    name: "shell",
    // `description` as well as `command`, because the shell tool requires it and a
    // row titles itself with it: without one the title falls back to the command
    // line, which is a shape production never sends and which this fixture is here
    // to keep out of the goldens.
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

// One call-scoped patch receipt for a created file. It deliberately has no line
// diff: the Runtime publishes path/status/from facts and the UI must not replace
// them with the current worktree.
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
    result: { changes: [{ path: "runtime/internal/session/atomicity.md", status: "added" }] },
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

// A patch that reports what it changed. Both halves are here on purpose: a path
// too long for the row (deep and absolute, which is what the runtime reports), so
// it MUST clip and which end it clips is visible — and a result carrying the
// Runtime's exact `{path,status}` receipt. Without an item like this, neither is
// in any golden; the waves state collapses tool rows into "N steps".
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
          path: "/Users/visual/scope/desktop/frontend/src/plugins/builtin/chat/tools/application/specialisedPreviewProjections.ts",
          status: "modified",
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

// A long turn: two rounds of work, each answered, then a third round still in flight.
// The two answered rounds fold to one row apiece; the live one stays open. Without a
// state shaped like this, nothing in the goldens ever showed the fold at all — and the
// fold is the whole reason a long turn stays readable.
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
  tool: { name: "read", arguments: { path: "runtime/internal/session/store.go" } },
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
  tool: { name: "grep", arguments: { pattern: "RunInTx", path: "runtime" } },
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
    result: { changes: [{ path: "runtime/internal/session/store.go", status: "modified" }] },
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

/** The RAW frame, before projection. See `RUNTIME_AGENT_SESSION_SNAPSHOTS` for why both. */
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

const LONG_RESPONSE = message(
  "agentMessage",
  "item_long_response",
  [
    "## Architecture review",
    "",
    "The consumer owns persistence policy and transaction scope. The Agent Framework remains reusable because it exposes execution capability without importing application records.",
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
    // A SECOND heading, so this answer has an outline and the end rail is
    // finally photographed at rest. One heading is the answer's own title and
    // the rail declines to draw it, which is why every golden until now framed
    // an empty end gutter — the one place a width regression there could hide.
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
  ].join("\n"),
);

// A multi-turn conversation with the full block vocabulary — the state the
// narrative rails and the block grammar are actually FOR. Every other fixture is
// one question and one answer, which is exactly the shape in which a turn map and
// an answer outline have nothing to say, so neither could be photographed.
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
    safetyClass: SAFETY_CLASS[name] ?? "exec",
    tool: { name, arguments: args, ...(result === undefined ? {} : { result }) },
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

/** The RAW wire form, before `runtimeItem` projects it. Exported so a test can hold it to
 *  the contract: the projected form below is a different shape by design, so validating that
 *  one proves nothing about what the Runtime would actually have sent. */
export const RUNTIME_AGENT_SESSION_SNAPSHOTS: Readonly<
  Record<VisualAgentState, RuntimeAgentSessionSnapshot>
> = {
  empty: BASE,
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
    items: [PROMPT, COMMENTARY_RESPONSE],
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
            detail: "The provider rejected the request. Verify the selected model and retry.",
          },
        },
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
  delegated: {
    runs: [
      run("running"),
      {
        id: "run_child",
        sessionId: SESSION_ID,
        status: "waiting",
        createdAt: "2026-07-31T08:00:03.000Z",
        metrics: { steps: 2, activeDurationMillis: 7_000 },
        protocolProfile: PROFILE,
        provider: RUN_PROVIDER,
        model: RUN_MODEL,
        parentRunId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: "item_delegate",
      },
      {
        id: "run_nested",
        sessionId: SESSION_ID,
        status: "running",
        activeSegmentId: "seg_nested",
        createdAt: "2026-07-31T08:00:05.000Z",
        metrics: { steps: 1, activeDurationMillis: 3_000 },
        protocolProfile: PROFILE,
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
          arguments: { summary: "Audit Agent Framework ownership", instructions: "…" },
        },
      },
      message(
        "agentMessage",
        "item_child_response",
        "The framework remains generic. I found no application persistence type in its production graph.",
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
  narrative: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:02:30.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [
      NARRATIVE_TURN_1,
      NARRATIVE_REASONING,
      // Four adjacent read-only calls, so the transcript photographs a tool GROUP
      // — a disclosure nested inside a disclosure. Every defect this shape has
      // shipped (a row overflowing its parent's rounded corner, an inner rail
      // with nowhere to go) survived because no fixture rendered one.
      narrativeTool("item_n_read", "read", { path: "src/checkout/checkout.tsx" }),
      narrativeTool("item_n_read_2", "read", {
        file_path:
          "/Users/visual/scope/desktop/frontend/src/plugins/builtin/chat/tools/ui/ToolGroup.tsx",
      }),
      narrativeTool("item_n_read_3", "read", { path: "src/checkout/api/pay.ts" }),
      narrativeTool("item_n_grep", "grep", { pattern: "retry|backoff", path: "src" }, "7 matches"),
      narrativeTool(
        "item_n_edit",
        "edit",
        { path: "src/checkout/hooks/useRetryPayment.ts" },
        "Created 85 lines",
      ),
      NARRATIVE_ANSWER_1,
      NARRATIVE_TURN_2,
      NARRATIVE_COMPACTION,
      narrativeTool("item_n_search", "web_search", { query: "stripe idempotency key retry" }),
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
    items: [PROMPT, GLOB_CALL, MEMORY_CALL, CONVERSATIONS_CALL, TOOL_SEARCH_CALL, RESPONSE],
    pendingInterruptSets: [],
  },

  "tool-shells": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
        // Six tool calls below, so the run's own count says six. The default
        // METRICS is four; leaving it would put two numbers that disagree in the
        // same frame, which is a bug everywhere except in a fixture nobody read.
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

export const AGENT_SESSION_SNAPSHOTS: Readonly<Record<VisualAgentState, AgentSessionSnapshot>> =
  Object.fromEntries(
    Object.entries(RUNTIME_AGENT_SESSION_SNAPSHOTS).map(([state, snapshot]) => [
      state,
      {
        ...snapshot,
        runs: snapshot.runs.map(runtimeRunFact),
        items: snapshot.items.map(runtimeItem),
        pendingInterruptSets: snapshot.pendingInterruptSets.map(runtimePendingInterruptSet),
      },
    ]),
  ) as Record<VisualAgentState, AgentSessionSnapshot>;

export const RUNTIME_AGENT_SESSION_TAIL_EVENTS: Readonly<Record<VisualAgentState, TailFrame[]>> = {
  empty: [],
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
  // One frame earlier than `running`: the answer is open and empty. Nothing here is
  // superseded yet, so the thinking stays readable and the live tool work stays
  // unfolded; an empty answer is not yet replacement material.
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
  recovery: [],
  delegated: [],
  "long-content": [],
  narrative: [],
  "tool-shells": [],
  "tool-search": [],
  "tool-remote": [],
  "tool-agentic": [],
  // The live round arrives as started items, not as snapshot history: a snapshot holds
  // only what has reached a terminal state.
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

/**
 * The session's standing order, for the states that have one.
 *
 * Beside the Agent inner-ring snapshot rather than inside it: Goal is companion
 * material from the same Runtime Session transaction, not an Item in the
 * transcript — which is why it gets a pinned banner instead of a card.
 *
 * `running` is deliberately the state that has one, so a golden frames the pinned
 * Goal and the composer-owned active Plan together.
 */
export const VISUAL_GOALS: Partial<Record<VisualAgentState, GoalReadModel>> = {
  running: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux without loosening any gate or skipping a test",
    status: "active",
    stop: null,
    // Cost is at 90% while runs is at 35%: the collapsed row must report the axis
    // that will stop the loop first, not the largest number on screen. Steps is
    // uncapped, so that limit is absent instead of encoded as a sentinel zero.
    budget: { maxRuns: 20, maxCostUsd: 5 },
    used: { runs: 7, costUsd: 4.5, steps: 31 },
    provider: "openai",
    model: "gpt-5",
    reasoningEffort: "high",
    createdAt: "2026-08-12T08:00:00Z",
    updatedAt: "2026-08-12T08:01:00Z",
  },
  terminal: {
    sessionId: SESSION_ID,
    objective: "Get the desktop suite green on Linux",
    status: "paused",
    stop: { code: "costBudgetReached", detail: "" },
    budget: { maxRuns: 20, maxCostUsd: 5 },
    used: { runs: 12, costUsd: 5, steps: 58 },
    provider: "openai",
    model: "gpt-5",
    reasoningEffort: "high",
    createdAt: "2026-08-12T08:00:00Z",
    updatedAt: "2026-08-12T08:02:00Z",
  },
};

export const VISUAL_SESSION_ID = SESSION_ID;
export const VISUAL_ROOT_RUN_ID = ROOT_RUN_ID;
