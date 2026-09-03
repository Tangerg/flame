import { describe, expect, it } from "vitest";

import {
  validateMethodResult,
  validateNotificationParams,
  validateWire,
} from "@flame/runtime-contract/validate";
import {
  MAXIMUM_EXACT_JSON_INTEGER,
  MAXIMUM_MODEL_IDENTITY_CHARACTERS,
  MAXIMUM_PAGINATION_CURSOR_CHARACTERS,
  MAXIMUM_PROVIDER_IDENTITY_CHARACTERS,
  MAXIMUM_REASONING_EFFORT_IDENTITY_CHARACTERS,
  MAXIMUM_RUNTIME_EVENT_SEQUENCE,
  MAXIMUM_RUN_EVENT_ID_CHARACTERS,
  RUN_EVENT_ID_PREFIX,
  runEventIsReplayable,
  runEventReliability,
  SESSION_ARTIFACT_VERSION,
} from "@flame/runtime-contract/wire";

// One case per rule the compiler translates, because each rule takes its own code
// path out of the schema tree: a type keyword, `required`, a closed `enum`, a value
// constraint stated beside a type and again alone in an allOf branch, union
// exclusivity, and a cross-field presence rule. A validator that agreed with the
// published schema on nine of them and silently dropped the tenth would still pass
// every canonical sample — the samples are valid frames, so only an invalid one
// proves a rule is enforced.

const session = {
  id: "ses_01",
  title: "Refactor the runtime protocol",
  status: "idle",
  provider: "anthropic",
  model: "claude-opus-4-8",
  workspace: {
    ref: { path: "/Users/dev/project" },
    projectRoot: "/Users/dev/project",
    availability: "available",
  },
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
  revision: 3,
};

describe("model identity wire constraints", () => {
  it("rejects control, whitespace, and overlong identities by Unicode code point", () => {
    const start = {
      sessionId: "ses_01",
      input: [{ type: "text", text: "go" }],
      provider: "openai",
      model: "gpt-5",
    };
    expect(validateWire("StartRunRequest", { ...start, provider: "open ai" })).toContainEqual({
      path: "StartRunRequest.provider",
      detail: expect.stringContaining("expected to match"),
    });
    expect(
      validateWire("StartRunRequest", {
        ...start,
        provider: "提".repeat(MAXIMUM_PROVIDER_IDENTITY_CHARACTERS),
        model: "模".repeat(MAXIMUM_MODEL_IDENTITY_CHARACTERS),
        reasoningEffort: "强".repeat(MAXIMUM_REASONING_EFFORT_IDENTITY_CHARACTERS),
      }),
    ).toEqual([]);
    expect(
      validateWire("StartRunRequest", {
        ...start,
        model: "m".repeat(MAXIMUM_MODEL_IDENTITY_CHARACTERS + 1),
      }),
    ).toContainEqual({
      path: "StartRunRequest.model",
      detail: `expected at most ${MAXIMUM_MODEL_IDENTITY_CHARACTERS} character(s)`,
    });
    expect(
      validateWire("ModelCapabilities", { reasoning: true, reasoningLevels: ["high\n"] }),
    ).toContainEqual({
      path: "ModelCapabilities.reasoningLevels[0]",
      detail: expect.stringContaining("expected to match"),
    });
    expect(validateWire("Usage", { byModel: { "bad model": {} } })).toContainEqual({
      path: 'Usage.byModel["bad model"]',
      detail: expect.stringContaining("expected to match"),
    });
    const overlongModel = "m".repeat(MAXIMUM_MODEL_IDENTITY_CHARACTERS + 1);
    expect(validateWire("ArtifactUsage", { byModel: { [overlongModel]: {} } })).toContainEqual({
      path: `ArtifactUsage.byModel[${JSON.stringify(overlongModel)}]`,
      detail: `expected at most ${MAXIMUM_MODEL_IDENTITY_CHARACTERS} character(s)`,
    });
  });
});

const artifactSession = {
  id: "ses_01",
  title: "Refactor the runtime protocol",
  provider: "anthropic",
  model: "claude-opus-4-8",
  workspace: { path: "/Users/dev/project" },
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
};

const finishedRun = {
  id: "run_01",
  sessionId: "ses_01",
  status: "finished",
  createdAt: "2026-07-07T10:00:00Z",
  finishedAt: "2026-07-07T10:00:20Z",
  metrics: { steps: 2, activeDurationMillis: 20_000 },
  protocolProfile: { requiredFeatures: [], interruptTypes: [] },
  provider: "openai",
  model: "gpt-5",
  outcome: { type: "completed" },
};

describe("the generated wire checks", () => {
  it("derives run-event reliability from the protocol event registry", () => {
    expect(runEventReliability("segment.finished")).toBe("authoritative");
    expect(runEventReliability("item.delta")).toBe("ephemeral");
    expect(runEventReliability("future.event")).toBeUndefined();
    expect(runEventIsReplayable("segment.finished")).toBe(true);
    expect(runEventIsReplayable("item.delta")).toBe(false);
    expect(runEventIsReplayable("future.event")).toBeUndefined();
  });

  it("accepts a well-formed frame", () => {
    expect(validateWire("Session", session)).toEqual([]);
  });

  it("requires at least one real segment-progress fact", () => {
    expect(validateWire("RunProgress", {})).toEqual([
      { path: "RunProgress", detail: "matches no permitted alternative" },
      { path: "RunProgress.step", detail: "is required" },
    ]);
    expect(validateWire("RunProgress", { activity: "Calling model" })).toEqual([]);
    expect(
      validateWire("RunProgress", {
        step: 0,
        contextTokens: 0,
        usage: { inputTokens: 0 },
      }),
    ).toEqual([]);
  });

  it("distinguishes an unwritten Plan from a committed empty replacement", () => {
    expect(validateWire("Plan", { sessionId: "ses_1" })).toEqual([]);
    expect(
      validateWire("Plan", {
        sessionId: "ses_1",
        state: { revision: 1, steps: [], updatedAt: "2026-08-29T00:00:00Z" },
      }),
    ).toEqual([]);
    expect(
      validateWire("StreamEvent", { type: "plan.updated", plan: { sessionId: "ses_1" } }),
    ).toContainEqual({ path: "StreamEvent.plan.state", detail: "is required" });
  });

  it("binds every method result to its registered wire shape", () => {
    expect(validateMethodResult("sessions.get", session)).toEqual([]);
    const { revision: _revision, ...malformed } = session;
    expect(validateMethodResult("sessions.get", malformed)).toEqual([
      { path: "sessions.get.result.revision", detail: "is required" },
    ]);
    expect(validateMethodResult("goals.get", null)).toEqual([]);
    expect(validateMethodResult("sessions.delete", {})).toEqual([]);
  });

  it("binds every downstream notification to its registered params shape", () => {
    expect(
      validateNotificationParams("notifications.runtime.event", {
        event: { type: "skills.changed", sequence: 1 },
      }),
    ).toEqual([]);
    expect(
      validateNotificationParams("notifications.runtime.event", {
        event: { type: "skills.changed", sequence: 0 },
      }),
    ).toContainEqual({
      path: "notifications.runtime.event.params.event.sequence",
      detail: "expected at least 1",
    });
  });

  it("names every missing required field", () => {
    const { id: _id, revision: _revision, ...partial } = session;
    expect(validateWire("Session", partial)).toEqual([
      { path: "Session.id", detail: "is required" },
      { path: "Session.revision", detail: "is required" },
    ]);
  });

  it("keeps start and resume acknowledgements semantically distinct", () => {
    expect(
      validateWire("StartRunResponse", {
        runId: "run_01",
        segmentId: "seg_01",
        userItemId: "item_01",
      }),
    ).toEqual([]);
    expect(validateWire("StartRunResponse", { runId: "run_01", segmentId: "seg_01" })).toEqual([
      { path: "StartRunResponse.userItemId", detail: "is required" },
    ]);
    expect(validateWire("ResumeRunResponse", { runId: "run_01", segmentId: "seg_02" })).toEqual([]);
    expect(
      validateWire("ResumeRunResponse", {
        runId: "run_01",
        segmentId: "seg_02",
        userItemId: "",
      }),
    ).toEqual([
      {
        path: "ResumeRunResponse.userItemId",
        detail: "expected at least 1 character(s)",
      },
    ]);
  });

  it("rejects a value of the wrong JSON type", () => {
    expect(validateWire("Session", { ...session, revision: "3" })).toEqual([
      { path: "Session.revision", detail: "expected an integer" },
    ]);
  });

  it("rejects a tag outside a closed value set", () => {
    const [violation] = validateWire("Session", { ...session, status: "sleeping" });
    expect(violation?.path).toBe("Session.status");
    expect(violation?.detail).toContain("expected one of");
  });

  // Shared result definitions stay open so an older client tolerates optional
  // fields added by a newer runtime. Request strictness is stated contextually by
  // OpenRPC and enforced by the runtime's request decoder.
  it("ignores a property the contract does not mention", () => {
    expect(validateWire("Session", { ...session, inventedByANewerServer: true })).toEqual([]);
  });

  it("rejects an empty string where the contract states a minimum length", () => {
    expect(validateWire("GetSessionRequest", { sessionId: "ses_01" })).toEqual([]);
    expect(validateWire("GetSessionRequest", { sessionId: "" })).toEqual([
      { path: "GetSessionRequest.sessionId", detail: "expected at least 1 character(s)" },
    ]);
  });

  // An omitted filter already means "every status", so the two ways of sending one
  // that means nothing — empty, or repeating a value — are the ones refused.
  it("rejects a filter array that is empty or repeats a value", () => {
    expect(validateWire("ListRunsRequest", {})).toEqual([]);
    expect(validateWire("ListRunsRequest", { statuses: ["running", "waiting"] })).toEqual([]);
    expect(validateWire("ListRunsRequest", { statuses: [] })).toEqual([
      { path: "ListRunsRequest.statuses", detail: "expected at least 1 item(s)" },
    ]);
    expect(validateWire("ListRunsRequest", { statuses: ["running", "running"] })).toEqual([
      { path: "ListRunsRequest.statuses", detail: "expected no repeated items" },
    ]);
  });

  it("rejects an empty secret-map replacement", () => {
    expect(
      validateWire("MCPHeadersChange", { type: "set", value: { "X-API-Key": "secret" } }),
    ).toEqual([]);
    expect(validateWire("MCPHeadersChange", { type: "set", value: {} })).toEqual([
      { path: "MCPHeadersChange.value", detail: "expected at least 1 property" },
    ]);
  });

  it("requires structured non-empty steering input", () => {
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [
          { type: "text", text: "compare this" },
          { type: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    ).toEqual([]);
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [],
      }),
    ).toEqual([{ path: "SteerRunRequest.input", detail: "expected at least 1 item(s)" }]);
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        message: "legacy",
      }),
    ).toEqual([{ path: "SteerRunRequest.input", detail: "is required" }]);
  });

  it("rejects a revision below the minimum", () => {
    expect(
      validateWire("UpdateSessionRequest", { sessionId: "ses_01", expectedRevision: 0 }),
    ).toEqual([{ path: "UpdateSessionRequest.expectedRevision", detail: "expected at least 1" }]);
  });

  it("enforces generated request bounds", () => {
    expect(validateWire("PageQuery", { limit: 0 })).toEqual([
      { path: "PageQuery.limit", detail: "expected at least 1" },
    ]);
    expect(
      validateWire("PageQuery", {
        cursor: "x".repeat(MAXIMUM_PAGINATION_CURSOR_CHARACTERS + 1),
      }),
    ).toEqual([
      {
        path: "PageQuery.cursor",
        detail: `expected at most ${MAXIMUM_PAGINATION_CURSOR_CHARACTERS} character(s)`,
      },
    ]);
    expect(
      validateWire("ListSessionsRequest", {
        cursor: "x".repeat(MAXIMUM_PAGINATION_CURSOR_CHARACTERS + 1),
      }),
    ).toEqual([
      {
        path: "ListSessionsRequest.cursor",
        detail: `expected at most ${MAXIMUM_PAGINATION_CURSOR_CHARACTERS} character(s)`,
      },
    ]);
    expect(
      validateMethodResult("sessions.list", {
        data: [],
        nextCursor: "x".repeat(MAXIMUM_PAGINATION_CURSOR_CHARACTERS + 1),
      }),
    ).toEqual([
      {
        path: "sessions.list.result.nextCursor",
        detail: `expected at most ${MAXIMUM_PAGINATION_CURSOR_CHARACTERS} character(s)`,
      },
    ]);
    expect(validateWire("ListSessionsRequest", { search: "x".repeat(1025) })).toEqual([
      { path: "ListSessionsRequest.search", detail: "expected at most 1024 character(s)" },
    ]);
    expect(
      validateWire("SubscribeRunResponse", {
        runId: "run_01",
        segmentId: "seg_01",
        headEventId: RUN_EVENT_ID_PREFIX + "x".repeat(MAXIMUM_RUN_EVENT_ID_CHARACTERS),
      }),
    ).toEqual([
      {
        path: "SubscribeRunResponse.headEventId",
        detail: `expected at most ${MAXIMUM_RUN_EVENT_ID_CHARACTERS} character(s)`,
      },
    ]);
    expect(
      validateWire("RunEvent", {
        runId: "run_01",
        segmentId: "seg_01",
        eventId: "opaque",
        timestamp: "2026-08-29T00:00:00Z",
        event: { type: "segment.progress", progress: { activity: "Working" } },
      }),
    ).toContainEqual({ path: "RunEvent.eventId", detail: "expected to match ^evt_" });
    expect(
      validateWire("SubscribeRunResponse", {
        runId: "run_01",
        segmentId: "seg_01",
        headEventId: "opaque",
      }),
    ).toContainEqual({
      path: "SubscribeRunResponse.headEventId",
      detail: "expected to match ^evt_",
    });
    expect(
      validateWire("SubscribeRunResponse", {
        runId: "run_01",
        segmentId: "seg_01",
        headEventId: "",
      }),
    ).toContainEqual({
      path: "SubscribeRunResponse.headEventId",
      detail: "expected to match ^evt_",
    });
    expect(validateWire("UsageSummaryRequest", { sinceDays: 0 })).toEqual([
      { path: "UsageSummaryRequest.sinceDays", detail: "expected at least 1" },
    ]);
    expect(validateWire("GenerationParams", { temperature: 2.1 })).toEqual([
      { path: "GenerationParams.temperature", detail: "expected at most 2" },
    ]);
    expect(validateWire("GenerationParams", { topP: 1.1 })).toEqual([
      { path: "GenerationParams.topP", detail: "expected at most 1" },
    ]);
    expect(
      validateWire("RuntimeEvent", {
        type: "skills.changed",
        sequence: MAXIMUM_RUNTIME_EVENT_SEQUENCE + 1,
      }),
    ).toContainEqual({
      path: "RuntimeEvent.sequence",
      detail: `expected at most ${MAXIMUM_RUNTIME_EVENT_SEQUENCE}`,
    });

    expect(
      validateWire("UpdateSessionRequest", {
        sessionId: "ses_01",
        expectedRevision: MAXIMUM_EXACT_JSON_INTEGER + 1,
      }),
    ).toContainEqual({
      path: "UpdateSessionRequest.expectedRevision",
      detail: `expected at most ${MAXIMUM_EXACT_JSON_INTEGER}`,
    });
    expect(
      validateWire("UpdateScheduleRequest", {
        id: "sch_01",
        expectedRevision: MAXIMUM_EXACT_JSON_INTEGER + 1,
      }),
    ).toContainEqual({
      path: "UpdateScheduleRequest.expectedRevision",
      detail: `expected at most ${MAXIMUM_EXACT_JSON_INTEGER}`,
    });
    expect(
      validateMethodResult("sessions.list", {
        data: [{ ...session, revision: MAXIMUM_EXACT_JSON_INTEGER + 1 }],
      }),
    ).toContainEqual({
      path: "sessions.list.result.data[0].revision",
      detail: `expected at most ${MAXIMUM_EXACT_JSON_INTEGER}`,
    });

    const boundaryReason = "😀".repeat(1024);
    expect(validateWire("CancelRunRequest", { runId: "run_01", reason: boundaryReason })).toEqual(
      [],
    );
    expect(
      validateWire("CancelRunRequest", { runId: "run_01", reason: `${boundaryReason}😀` }),
    ).toEqual([
      {
        path: "CancelRunRequest.reason",
        detail: "expected at most 1024 character(s)",
      },
    ]);

    const boundaryMemory = "😀".repeat(4096);
    expect(
      validateWire("AgentMemoryAddRequest", { scope: "user", content: boundaryMemory }),
    ).toEqual([]);
    expect(
      validateWire("AgentMemoryUpdateRequest", {
        id: "mem_0123456789abcdef0123456789abcdef",
        content: `${boundaryMemory}😀`,
      }),
    ).toEqual([
      {
        path: "AgentMemoryUpdateRequest.content",
        detail: "expected at most 4096 character(s)",
      },
    ]);
    expect(
      validateMethodResult("agentMemory.add", {
        id: "mem_0123456789abcdef0123456789abcdef",
        scope: "user",
        content: `${boundaryMemory}😀`,
        origin: "user",
        status: "active",
        pinned: false,
        createdAt: "2026-08-24T00:00:00Z",
        updatedAt: "2026-08-24T00:00:00Z",
      }),
    ).toContainEqual({
      path: "agentMemory.add.result.content",
      detail: "expected at most 4096 character(s)",
    });
  });

  // The constraint belongs to this request, not to every carrier of the shared
  // shape, so the schema states it in an allOf branch — a third code path, and the
  // one that reads `minLength` with no type keyword beside it.
  it("states a constraint on a field of a shared shape", () => {
    const artifact = {
      version: SESSION_ARTIFACT_VERSION,
      session: artifactSession,
      items: [],
      messages: [],
      runs: [],
      toolResults: [],
    };
    expect(validateWire("ImportSessionRequest", { artifact })).toEqual([]);
    expect(
      validateWire("ImportSessionRequest", {
        artifact: { ...artifact, session: { ...artifactSession, id: "" } },
      }),
    ).toEqual([
      {
        path: "ImportSessionRequest.artifact.session.id",
        detail: "expected at least 1 character(s)",
      },
    ]);
  });

  it("accepts one variant of a union and refuses another variant's field", () => {
    expect(validateWire("ContentBlock", { type: "text", text: "hello" })).toEqual([]);
    expect(
      validateWire("ContentBlock", { type: "text", text: "hello", mime: "image/png" }),
    ).toEqual([
      { path: "ContentBlock", detail: "matches no permitted variant" },
      { path: "ContentBlock.mime", detail: "must not be present here" },
    ]);
  });

  it("narrows Item status by variant and stream lifecycle", () => {
    const userItem = {
      type: "userMessage",
      id: "item_user",
      runId: "run_01",
      createdAt: "2026-08-29T11:00:00Z",
      content: [{ type: "text", text: "hello" }],
    };
    expect(validateWire("Item", { ...userItem, status: "completed" })).toEqual([]);
    expect(validateWire("Item", { ...userItem, status: "running" })).toContainEqual({
      path: "Item.status",
      detail: 'expected one of "completed"',
    });

    const runningAgent = {
      type: "agentMessage",
      id: "item_agent",
      runId: "run_01",
      createdAt: "2026-08-29T11:00:00Z",
      status: "running",
    };
    expect(validateWire("StreamEvent", { type: "item.started", item: runningAgent })).toEqual([]);
    expect(
      validateWire("StreamEvent", { type: "item.completed", item: runningAgent }),
    ).toContainEqual({
      path: "StreamEvent.item.status",
      detail: 'expected one of "completed", "incomplete"',
    });

    expect(
      validateWire("ArtifactItem", {
        type: "toolCall",
        id: "item_tool",
        runId: "run_01",
        startedAt: "2026-08-29T11:00:00Z",
        status: "running",
        tool: { name: "shell", arguments: { command: "pwd" } },
      }),
    ).toContainEqual({
      path: "ArtifactItem.status",
      detail: 'expected one of "completed", "incomplete"',
    });
  });

  it("keeps cancel root and child results closed and distinct", () => {
    const canceledRoot = { ...finishedRun, outcome: { type: "canceled" } };
    const canceledChild = {
      ...canceledRoot,
      id: "run_child",
      spawnedByItemId: "item_parent",
      parentRunId: "run_01",
      rootRunId: "run_01",
    };
    expect(validateWire("CancelRunResponse", { type: "root", run: canceledRoot })).toEqual([]);
    expect(
      validateWire("CancelRunResponse", {
        type: "child",
        run: canceledChild,
        rootRun: canceledRoot,
      }),
    ).toEqual([]);
    expect(
      validateWire("CancelRunResponse", {
        type: "root",
        run: canceledRoot,
        rootRun: canceledRoot,
      }),
    ).toEqual([
      { path: "CancelRunResponse", detail: "matches no permitted variant" },
      { path: "CancelRunResponse.rootRun", detail: "must not be present here" },
    ]);
    expect(validateWire("CancelRunResponse", { type: "child", run: canceledChild })).toEqual([
      { path: "CancelRunResponse", detail: "matches no permitted variant" },
      { path: "CancelRunResponse.rootRun", detail: "is required" },
    ]);
  });

  // The scope of a read is a union for the same reason a content block is: a frame
  // carrying both subjects would need a precedence rule to resolve, and the flag only
  // means something where there is a subtree to include.
  it("keeps a read's two scopes exclusive", () => {
    expect(validateWire("ItemListScope", { type: "session", sessionId: "ses_01" })).toEqual([]);
    expect(
      validateWire("ItemListScope", { type: "run", runId: "run_01", includeDescendants: true }),
    ).toEqual([]);
    expect(
      validateWire("ItemListScope", { type: "session", sessionId: "ses_01", runId: "run_01" }),
    ).toEqual([
      { path: "ItemListScope", detail: "matches no permitted variant" },
      { path: "ItemListScope.runId", detail: "must not be present here" },
    ]);
    expect(
      validateWire("ItemListScope", {
        type: "session",
        sessionId: "ses_01",
        includeDescendants: true,
      }),
    ).toEqual([
      { path: "ItemListScope", detail: "matches no permitted variant" },
      { path: "ItemListScope.includeDescendants", detail: "must not be present here" },
    ]);
  });

  it("refuses a discriminator no variant claims", () => {
    const details = validateWire("ContentBlock", { type: "video", data: "AAAA" }).map(
      (v) => v.detail,
    );
    expect(details).toContain("matches no permitted variant");
  });

  // A rule declared for RunSummary has to reach the RunRef that embeds it: the
  // fields are inlined onto one frame, so a rule that stopped at the summary would
  // leave the shape a client actually receives unchecked.
  it("applies an embedded shape's rules to the shape embedding it", () => {
    const { parentRunId: _parent, ...rootChild } = {
      ...finishedRun,
      spawnedByItemId: "item_03",
      parentRunId: "run_02",
      rootRunId: "run_02",
    };
    // Named ONCE, though the rule is stated per edge and both surviving edges
    // independently demand the missing field. How many edges happen to state a
    // rule is a fact about how the schema is factored, not about the frame.
    expect(validateWire("RunRef", rootChild)).toEqual([
      { path: "RunRef.parentRunId", detail: "is required" },
    ]);
    // The counter-example: all three edges together is what a child looks like.
    expect(validateWire("RunRef", { ...rootChild, parentRunId: "run_02" })).toEqual([]);
  });

  it("enforces a cross-field presence rule", () => {
    expect(validateWire("RunRef", finishedRun)).toEqual([]);
    const { outcome: _outcome, ...unexplained } = finishedRun;
    expect(validateWire("RunRef", unexplained)).toEqual([
      { path: "RunRef.outcome", detail: "is required" },
    ]);

    expect(
      validateWire("StartRunRequest", {
        sessionId: "ses_01",
        input: [{ type: "text", text: "go" }],
        provider: "provider",
      }),
    ).toContainEqual({ path: "StartRunRequest.model", detail: "is required" });
    expect(
      validateWire("UpdateSessionRequest", {
        sessionId: "ses_01",
        expectedRevision: 1,
        model: "model",
      }),
    ).toContainEqual({ path: "UpdateSessionRequest.provider", detail: "is required" });
  });

  it("checks array elements and names the index", () => {
    expect(validateWire("PageOfSession", { data: [session, session] })).toEqual([]);
    expect(
      validateWire("PageOfSession", { data: [session, { ...session, revision: "3" }] }),
    ).toEqual([{ path: "PageOfSession.data[1].revision", detail: "expected an integer" }]);
  });

  it("keeps the run-event opt-out vocabulary narrower than the event union", () => {
    expect(
      validateWire("ClientCapabilities", {
        excludedEphemeralEvents: ["segment.progress", "item.delta"],
      }),
    ).toEqual([]);
    expect(
      validateWire("ClientCapabilities", {
        excludedEphemeralEvents: ["item.completed"],
      }),
    ).toEqual([
      {
        path: "ClientCapabilities.excludedEphemeralEvents[0]",
        detail: 'expected one of "segment.progress", "item.delta"',
      },
    ]);
  });

  it("enforces output value constraints from the same machine contract", () => {
    expect(validateWire("RuntimeEvent", { type: "skills.changed", sequence: 0 })).toEqual([
      { path: "RuntimeEvent.sequence", detail: "expected at least 1" },
    ]);
    expect(
      validateWire("RuntimeEvent", {
        type: "files.changed",
        sequence: 1,
        paths: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.paths",
      detail: "expected at least 1 item(s)",
    });
    expect(validateWire("RuntimeEvent", { type: "resync", sequence: 1 })).toContainEqual({
      path: "RuntimeEvent.topics",
      detail: "is required",
    });
    expect(
      validateWire("RuntimeEvent", {
        type: "resync",
        sequence: 1,
        topics: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.topics",
      detail: "expected at least 1 item(s)",
    });
    expect(
      validateWire("RuntimeEvent", {
        type: "sessions.changed",
        sequence: 1,
        sessionIds: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.sessionIds",
      detail: "expected at least 1 item(s)",
    });

    expect(
      validateWire("RuntimeLimits", {
        runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
      }),
    ).toEqual([
      { path: "RuntimeLimits.idempotency", detail: "is required" },
      { path: "RuntimeLimits.mcpAuthorizationAttempts", detail: "is required" },
      { path: "RuntimeLimits.runReplay", detail: "is required" },
    ]);
    expect(
      validateWire("IdempotencyLimits", {
        namespace: "idp_fedcba9876543210fedcba9876543210",
        retentionSeconds: 0,
      }),
    ).toEqual([{ path: "IdempotencyLimits.retentionSeconds", detail: "expected at least 1" }]);
    expect(validateWire("IdempotencyLimits", { namespace: "", retentionSeconds: 1 })).toEqual([
      {
        path: "IdempotencyLimits.namespace",
        detail: "expected to match ^idp_[0-9a-f]{32}$",
      },
    ]);
    expect(validateWire("MCPAuthorizationAttemptLimits", { retentionSeconds: 0 })).toEqual([
      { path: "MCPAuthorizationAttemptLimits.retentionSeconds", detail: "expected at least 1" },
    ]);
    expect(
      validateWire("PendingInterruptSet", {
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toContainEqual({
      path: "PendingInterruptSet.interrupts",
      detail: "expected at least 1 item(s)",
    });
    expect(
      validateWire("PendingInterruptSet", {
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [
          {
            type: "question",
            itemId: "item_01",
            runId: "",
            payload: {
              question: { fields: [{ type: "text", prompt: "Continue?" }] },
            },
          },
        ],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toContainEqual({
      path: "PendingInterruptSet.interrupts[0].runId",
      detail: "expected at least 1 character(s)",
    });
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "choice",
            prompt: "Choose",
            options: [{ label: "A" }, { label: "B" }],
            allowCustom: true,
          },
        ],
      }),
    ).toEqual([]);
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "choice",
            prompt: "Choose",
            header: "😀".repeat(12),
            options: [{ label: "A" }],
          },
        ],
      }),
    ).toContainEqual({
      path: "Question.fields[0].options",
      detail: "expected at least 2 item(s)",
    });
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "text",
            prompt: "Explain",
            header: "😀".repeat(13),
          },
        ],
      }),
    ).toContainEqual({
      path: "Question.fields[0].header",
      detail: "expected at most 12 character(s)",
    });
    expect(
      validateWire("InterruptResponseValue", {
        type: "answer",
        answers: { q0: ["A"] },
      }),
    ).toContainEqual({
      path: "InterruptResponseValue.answers",
      detail: "expected an array",
    });
    expect(
      validateWire("ProblemData", {
        type: "capability_not_negotiated",
        requiredCapabilities: [],
      }),
    ).toContainEqual({
      path: "ProblemData.requiredCapabilities",
      detail: "expected at least 1 item(s)",
    });
    const repeatedRequirement = { type: "feature", name: "subagents" };
    expect(
      validateWire("ProblemData", {
        type: "capability_not_negotiated",
        requiredCapabilities: [repeatedRequirement, repeatedRequirement],
      }),
    ).toContainEqual({
      path: "ProblemData.requiredCapabilities",
      detail: "expected no repeated items",
    });

    expect(validateWire("ProblemData", { type: "run_lost" })).toEqual([]);
    expect(
      validateWire("ProblemData", { type: "mcp_dial_failed", detail: "connection failed" }),
    ).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "plugin:acme/model_timeout",
        retryAfterSeconds: 2,
      }),
    ).toEqual([]);
    expect(validateWire("ProblemData", { type: "model_timeout" })).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(validateWire("ProblemData", { type: "plugin:Acme/model_timeout" })).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "run_lost",
        activeRun: { runId: "run_1", status: "running" },
      }),
    ).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "idempotency_in_progress",
        retryAfterSeconds: 0,
      }),
    ).toContainEqual({
      path: "ProblemData.retryAfterSeconds",
      detail: "expected at least 1",
    });
  });
});
