import { describe, expect, it } from "vitest";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createFlameClient } from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import type { AgentSessionSummary } from "../application/session/sessionQueries";
import type { TrajectoryPage } from "../application/run/trajectory";
import { registerAgentDataProviders } from "./runtimeDataProviders";

async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createFlameClient(t);
  try {
    await contributeForTest((ctx) => registerAgentDataProviders(ctx, () => client));

    const fetcher = lookupDataProvider<T>(key);
    if (!fetcher) throw new Error(`no provider for "${key}"`);
    const pending = fetcher(params);
    const requests: Array<{ method: string; params: unknown }> = [];
    for (const [method, result] of responses) {
      const req = await waitForRequest(t, method);
      requests.push({ method: req.method, params: req.params });
      respondSuccess(t, req.id, result);
    }
    return { value: await pending, requests };
  } finally {
    await client.close();
  }
}

describe("agent Runtime data providers", () => {
  it("rejects missing parameters before a parameterized provider reaches RPC", async () => {
    const client = createFlameClient(createMemoryTransport());
    try {
      await contributeForTest((ctx) => registerAgentDataProviders(ctx, () => client));

      for (const key of ["session-trajectory", "trajectory-run", "approval-rules"]) {
        const fetcher = lookupDataProvider(key);
        expect(fetcher).toBeDefined();
        await expect(fetcher!()).rejects.toThrow(`Data provider "${key}" requires parameters`);
      }
    } finally {
      await client.close();
    }
  });

  it("projects a durable trajectory page without loading its next cursor", async () => {
    const query = { sessionId: "ses_1", includeDescendants: true, limit: 100, cursor: "older" };
    const occurredAt = "2026-09-28T00:00:00Z";
    const { value, requests } = await runProvider<TrajectoryPage>(
      "session-trajectory",
      [
        [
          "sessions.trajectory",
          {
            nextCursor: "earlier",
            data: [
              {
                type: "model",
                occurredAt,
                model: {
                  callId: "call_1",
                  runId: "run_1",
                  segmentId: "seg_1",
                  state: "unknown",
                  startedAt: occurredAt,
                  settledAt: "2026-09-28T00:00:03Z",
                },
              },
              {
                type: "item",
                occurredAt,
                item: {
                  id: "item_1",
                  runId: "run_1",
                  type: "toolCall",
                  status: "incomplete",
                  startedAt: occurredAt,
                  finishedAt: "2026-09-28T00:00:02Z",
                  durationMillis: 0,
                  approvalDecision: "deny",
                  tool: { name: "shell", arguments: {}, result: {} },
                  error: { type: "denied_by_user", detail: "denied" },
                },
              },
            ],
          },
        ],
      ],
      query,
    );

    expect(requests).toEqual([{ method: "sessions.trajectory", params: query }]);
    expect(value.nextCursor).toBe("earlier");
    expect(value.data[0]).toEqual({
      type: "model",
      occurredAt,
      model: {
        callId: "call_1",
        runId: "run_1",
        segmentId: "seg_1",
        state: "unknown",
        startedAt: occurredAt,
        settledAt: "2026-09-28T00:00:03Z",
      },
    });
    expect(value.data[1]).toMatchObject({
      type: "item",
      item: {
        durationMillis: 0,
        approvalDecision: "deny",
        error: { code: "denied_by_user" },
        tool: { result: {} },
      },
    });
  });

  it("sessions: maps Page<Session>.data into AgentSessionSummary rows (updatedAt → time)", async () => {
    const { value: rows } = await runProvider<AgentSessionSummary[]>("sessions", [
      [
        "sessions.list",
        {
          data: [
            {
              id: "ses_1",
              revision: 7,
              title: "Refactor auth",
              status: "running",
              provider: "anthropic",
              model: "claude",
              reasoningEffort: "high",
              workspace: {
                ref: { path: "/work/auth" },
                projectRoot: "/work/auth",
                availability: "missing",
              },
              createdAt: "2026-06-01T00:00:00Z",
              updatedAt: "2026-06-01T01:00:00Z",
            },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "ses_1",
        revision: 7,
        title: "Refactor auth",
        status: "running",
        provider: "anthropic",
        model: "claude",
        reasoningEffort: "high",
        workspace: { path: "/work/auth", availability: "missing" },
        time: "2026-06-01T01:00:00Z",
      },
    ]);
  });
});
