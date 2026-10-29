import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ErrorBoundary } from "react-error-boundary";
import type { ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import discovery from "@flame/runtime-client/testing/samples/method.discover.resp.json";
import type { Page, TrajectoryEntry } from "@flame/runtime-client/wire";
import { cloneJSON, parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { createTestClient } from "@/test/renderClient";
import { Trajectory } from "../../../plugins/trajectory/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS, VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";

afterEach(cleanup);

it.each([
  ["arguments", "9007199254740993"],
  ["arguments", "0.1000000000000000000001"],
  ["result", "18446744073709551615"],
  ["result", "1e999999999"],
] as const)("reads, searches and expands exact Tool %s containing %s", async (field, literal) => {
  const source = `{"value":${literal},"negativeZero":-0,"data":{"中文":"保留两处  空格","rows":[null,true,0.25]}}`;
  const payload = parseJSON(source) as Record<string, unknown>;
  const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS.idle;
  const page: Page<TrajectoryEntry> = {
    data: [
      {
        type: "item",
        occurredAt: "2026-07-31T08:00:05.999Z",
        item: {
          type: "toolCall",
          id: "item_exact_evidence",
          runId: snapshot.runs[0]!.id,
          status: "completed",
          startedAt: "2026-07-31T08:00:05.999Z",
          finishedAt: "2026-07-31T08:00:06.250Z",
          durationMillis: 251,
          tool: {
            name: "inspect_exact_records",
            arguments: field === "arguments" ? payload : { query: "ordinary" },
            ...(field === "result" ? { result: payload } : {}),
          },
        },
      },
      { type: "run", occurredAt: snapshot.runs[0]!.createdAt, run: snapshot.runs[0]! },
    ],
  };
  const captured = stringifyJSON(page);
  const transport = createMemoryTransport();
  const runtime = createFlameClient(
    {
      recv: transport.recv,
      close: transport.close,
      async send(request, signal, options) {
        await transport.send(request, signal, options);
        const result =
          request.method === "runtime.discover"
            ? { ...discovery, capabilities: VISUAL_CAPABILITIES }
            : request.method === "sessions.trajectory"
              ? cloneJSON(page)
              : undefined;
        if (!result) throw new Error(`Unexpected read: ${request.method}`);
        transport.inject({ jsonrpc: "2.0", id: request.id, result });
      },
    },
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const owner = new AbortController();
  const client = createTestClient();
  const context: ComponentContext = {
    component: { installationId: "trajectory", digest: "c".repeat(64), componentId: "trajectory" },
    runtime,
    sessionId: VISUAL_SESSION_ID,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    compose: () => null,
  };
  const failures: unknown[] = [];
  try {
    const view = client.render(
      <ErrorBoundary
        fallback={<p role="alert">Recorded page failed</p>}
        onError={(error) => failures.push(error)}
      >
        <Trajectory context={context} />
      </ErrorBoundary>,
    );
    await waitFor(() => {
      expect(
        screen.queryByRole("alert") !== null ||
          view.container.querySelectorAll("[data-trajectory-record]").length === 2,
      ).toBe(true);
    });
    expect(failures).toEqual([]);
    expect(screen.queryByRole("alert")?.textContent).toBeUndefined();
    expect(screen.getByText("Page 1 · 2 records")).toBeTruthy();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter this page" });
    search.focus();
    fireEvent.change(search, { target: { value: literal } });
    expect(screen.getByText("Page 1 · 1 of 2 records")).toBeTruthy();
    const record = within(view.container.querySelector<HTMLElement>("[data-trajectory-record]")!);
    const trigger = record.getByRole("button", { expanded: false });
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    const raw = record.getByRole("region", { name: "Recorded evidence" }).textContent!;
    expect(stringifyJSON(parseJSON(raw))).toBe(stringifyJSON(page.data[0]));
    expect(raw).toContain(`\n        "value": ${literal},\n        "negativeZero": -0,`);
    expect(raw).toContain('"中文": "保留两处  空格"');
    expect(raw).not.toContain(`"value": "${literal}"`);

    search.focus();
    fireEvent.change(search, { target: { value: "保留两处  空格" } });
    expect(document.activeElement).toBe(search);
    expect(record.getByRole("button", { expanded: true })).toBe(trigger);
    fireEvent.change(search, { target: { value: "" } });
    expect(screen.getByText("Page 1 · 2 records")).toBeTruthy();
    expect(record.getByRole("button", { expanded: true })).toBe(trigger);
    expect(transport.outbox().map((request) => request.method)).toEqual([
      "runtime.discover",
      "sessions.trajectory",
    ]);
    expect(stringifyJSON(page)).toBe(captured);
  } finally {
    cleanup();
    owner.abort();
    await runtime.close();
    client.dispose();
  }
});
