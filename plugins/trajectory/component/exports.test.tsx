// @vitest-environment happy-dom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type {
  ClientCommand,
  ComponentContext,
  FileSaveResult,
} from "@flame/component-sdk";
import type { FlameClient } from "@flame/runtime-client";
import type {
  ExportSessionResponse,
  ExportTrajectoryResponse,
} from "@flame/runtime-client/wire";
import { parseJSON } from "@qimu/sdk/json";
import { SessionExports, saveSessionExport } from "./exports";

afterEach(cleanup);

const artifactText =
  '{"session":{"id":"captured"},"evidence":{"count":9007199254740993,"fraction":0.1000000000000000000001}}';
const trajectory = {
  schemaVersion: 1,
  collectedAt: "2026-09-28T00:00:00Z",
  session: { id: "captured" },
  runs: [{ id: "child" }],
  modelInvocations: [{ callId: "unknown", state: "unknown" }],
  feedback: [{ rating: "negative", text: "incorrect result" }],
  limitations: ["retained evidence"],
};

function fixture() {
  const controller = new AbortController();
  const exportQuery = vi.fn(
    async (
      _id: string,
      format?: string,
      _signal?: AbortSignal,
    ): Promise<ExportSessionResponse> =>
      format === "md"
        ? { format: "md", markdown: "# Résumé 🔥\n" }
        : {
            format: "json",
            artifact: parseJSON(
              artifactText,
            ) as ExportSessionResponse["artifact"],
          },
  );
  const trajectoryQuery = vi.fn(
    async (_request: unknown, _signal?: AbortSignal) =>
      ({ trajectory }) as unknown as ExportTrajectoryResponse,
  );
  const saveFile = vi
    .fn<ComponentContext["saveFile"]>()
    .mockResolvedValue("saved");
  const commands = new Map<string, ClientCommand>();
  const context: ComponentContext = {
    component: {
      installationId: "installed",
      digest: "a".repeat(64),
      componentId: "exports",
    },
    runtime: {
      sessions: { export: exportQuery, exportTrajectory: trajectoryQuery },
    } as unknown as FlameClient,
    sessionId: "visible-other",
    locale: "en",
    signal: controller.signal,
    telemetry: {
      read(): never {
        throw new Error("Exports must not read client telemetry");
      },
      clear(): never {
        throw new Error("Exports must not clear client telemetry");
      },
    },
    track(operation) {
      controller.signal.throwIfAborted();
      return operation(controller.signal);
    },
    saveFile,
    compose() {
      throw new Error("Exports must not compose a view");
    },
    registerCommand(command) {
      if (commands.has(command.id)) throw new Error("duplicate command");
      commands.set(command.id, command);
      return () => {
        commands.delete(command.id);
      };
    },
    subscribeRootRunSettlements() {
      throw new Error("Exports must not observe runs");
    },
    notifications: {
      applicationName: "Test application",
      authorization() {
        throw new Error("Exports must not request notifications");
      },
      subscribe() {
        throw new Error("Exports must not request notifications");
      },
      refreshAuthorization() {
        throw new Error("Exports must not request notifications");
      },
      requestAuthorization() {
        throw new Error("Exports must not request notifications");
      },
      send() {
        throw new Error("Exports must not request notifications");
      },
    },
  };
  return {
    context,
    controller,
    exportQuery,
    trajectoryQuery,
    saveFile,
    commands,
  };
}

it("exports the complete Runtime artifact with its exact opaque numeric values", async () => {
  const { context, exportQuery, saveFile } = fixture();
  await saveSessionExport(context, "captured", "json", context.signal);
  expect(exportQuery).toHaveBeenCalledExactlyOnceWith(
    "captured",
    "json",
    context.signal,
  );
  const request = saveFile.mock.calls[0]![0];
  expect(request.filename).toMatch(/^flame-captured-.*\.json$/);
  expect(request.mime).toBe("application/json;charset=utf-8");
  expect(new TextDecoder().decode(request.bytes)).toBe(artifactText);
});

it("saves the complete trajectory envelope rather than the visible page", async () => {
  const { context, trajectoryQuery, exportQuery, saveFile } = fixture();
  await saveSessionExport(context, "captured", "trajectory", context.signal);
  expect(trajectoryQuery).toHaveBeenCalledExactlyOnceWith(
    { sessionId: "captured" },
    context.signal,
  );
  expect(exportQuery).not.toHaveBeenCalled();
  const request = saveFile.mock.calls[0]![0];
  expect(request.filename).toMatch(/^flame-captured-trajectory-.*\.json$/);
  expect(JSON.parse(new TextDecoder().decode(request.bytes))).toEqual(
    trajectory,
  );
});

it("transfers the Runtime's Markdown as exact UTF-8 bytes with a suggested basename", async () => {
  const { context, exportQuery, saveFile } = fixture();
  await saveSessionExport(context, "captured", "md", context.signal);
  expect(exportQuery).toHaveBeenCalledExactlyOnceWith(
    "captured",
    "md",
    context.signal,
  );
  expect(saveFile).toHaveBeenCalledExactlyOnceWith({
    filename: expect.stringMatching(/^flame-captured-.*\.md$/),
    mime: "text/markdown;charset=utf-8",
    bytes: new TextEncoder().encode("# Résumé 🔥\n"),
  });
});

it.each(["md", "json", "trajectory"] as const)(
  "waits for the accepted %s native file handoff",
  async (format) => {
    const { context, saveFile } = fixture();
    const choice = Promise.withResolvers<FileSaveResult>();
    saveFile.mockReturnValueOnce(choice.promise);
    let settled = false;
    const saving = saveSessionExport(
      context,
      "captured",
      format,
      context.signal,
    ).then(() => {
      settled = true;
    });
    try {
      await vi.waitFor(() => expect(saveFile).toHaveBeenCalledOnce());
      expect(settled).toBe(false);
    } finally {
      choice.resolve("saved");
      await saving;
    }
  },
);

it.each(["md", "json", "trajectory"] as const)(
  "preserves a %s save failure and treats cancellation as an ordinary result",
  async (format) => {
    const { context, saveFile } = fixture();
    const failure = new Error("selected file could not be written");
    saveFile.mockRejectedValueOnce(failure).mockResolvedValueOnce("cancelled");
    await expect(
      saveSessionExport(context, "captured", format, context.signal),
    ).rejects.toBe(failure);
    await expect(
      saveSessionExport(context, "captured", format, context.signal),
    ).resolves.toBeUndefined();
  },
);

it.each(["md", "json", "trajectory"] as const)(
  "refuses a late %s save after its installed source is withdrawn",
  async (format) => {
    const { context, controller, exportQuery, trajectoryQuery, saveFile } =
      fixture();
    const response = Promise.withResolvers<ExportSessionResponse>();
    const evidence = Promise.withResolvers<ExportTrajectoryResponse>();
    if (format === "trajectory")
      trajectoryQuery.mockReturnValueOnce(evidence.promise);
    else exportQuery.mockReturnValueOnce(response.promise);
    const exporting = saveSessionExport(
      context,
      "captured",
      format,
      context.signal,
    );
    controller.abort(new Error("source retired"));
    response.resolve({
      format: "md",
      markdown: "retired",
      artifact: parseJSON(artifactText) as ExportSessionResponse["artifact"],
    });
    evidence.resolve({ trajectory } as unknown as ExportTrajectoryResponse);
    await expect(exporting).rejects.toBe(context.signal.reason);
    expect(saveFile).not.toHaveBeenCalled();
  },
);

it.each(["md", "json", "trajectory"] as const)(
  "preserves a refused %s capture without saving a partial substitute",
  async (format) => {
    const { context, exportQuery, trajectoryQuery, saveFile } = fixture();
    const failure = new Error("session is busy");
    if (format === "trajectory") trajectoryQuery.mockRejectedValueOnce(failure);
    else exportQuery.mockRejectedValueOnce(failure);
    await expect(
      saveSessionExport(context, "captured", format, context.signal),
    ).rejects.toBe(failure);
    expect(saveFile).not.toHaveBeenCalled();
  },
);

it("refuses a malformed canonical export instead of inventing an empty result", async () => {
  const { context, exportQuery, saveFile } = fixture();
  exportQuery.mockResolvedValueOnce({ format: "json" });
  await expect(
    saveSessionExport(context, "captured", "json", context.signal),
  ).rejects.toThrow("omitted its json result");
  expect(saveFile).not.toHaveBeenCalled();
});

it("contributes localized commands for its mount and does not read without an invoked Session", async () => {
  const { context, commands, exportQuery, trajectoryQuery, saveFile } =
    fixture();
  const mounted = render(<SessionExports context={context} />);
  expect([...commands.values()].map((command) => command.label)).toContain(
    "Export complete Session trajectory",
  );
  for (const command of commands.values())
    await command.run({ signal: context.signal });
  expect(exportQuery).not.toHaveBeenCalled();
  expect(trajectoryQuery).not.toHaveBeenCalled();
  expect(saveFile).not.toHaveBeenCalled();
  await act(() =>
    mounted.rerender(<SessionExports context={{ ...context, locale: "zh" }} />),
  );
  expect(commands.get("trajectory")!.label).toBe("导出完整会话轨迹");
  await commands
    .get("markdown")!
    .run({ sessionId: "captured", signal: context.signal });
  expect(exportQuery).toHaveBeenCalledExactlyOnceWith(
    "captured",
    "md",
    context.signal,
  );
  mounted.unmount();
  expect(commands.size).toBe(0);
});
