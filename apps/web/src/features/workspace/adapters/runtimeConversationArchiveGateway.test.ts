import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { parseJSON } from "@flame/runtime-client/json";
import {
  exportConversationJson,
  exportConversationMarkdown,
  exportSessionTrajectory,
} from "@/features/workspace/application/conversationExport";
import { installConversationArchiveGateway } from "@/features/workspace/adapters/runtimeConversationArchiveGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
import { createFileTransfer } from "@/features/workspace/adapters/fileTransfer";
import { createDesktopHostClient } from "@/platform/desktopHost";
import { notifyError } from "@/features/notifications/public/notifications";
const mocks = vi.hoisted(() => ({
  download: vi.fn<(filename: string, content: string, mime: string) => Promise<void>>(),
}));

vi.mock("@/features/connection/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

vi.mock("@/features/agent/public/session", () => ({
  getActiveSessionId: () => "session-current",
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
  rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  selectAgentSession: vi.fn(),
}));

vi.mock("@/extensions/selectors/extensions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/selectors/extensions")>();
  return { ...actual, lookupExtensionByKey: vi.fn() };
});
vi.mock("@/features/notifications/public/notifications", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/notifications/public/notifications")>();
  return { ...actual, notifyError: vi.fn() };
});

const files: FileTransferPort = { saveText: mocks.download, pickText: vi.fn() };

const installations: Array<ReturnType<typeof installConversationArchiveGateway>> = [];

beforeEach(() => {
  mocks.download.mockReset().mockResolvedValue(undefined);
  vi.mocked(notifyError).mockReset();
});

afterEach(async () => {
  for (const installation of installations.splice(0).reverse()) installation.dispose();
  vi.restoreAllMocks();
});

describe("runtimeConversationArchiveGateway", () => {
  it("preserves exact opaque numbers through the injected export destination", async () => {
    const artifact = parseJSON(
      '{"session":{"id":"session-current"},"evidence":{"count":9007199254740993,"fraction":0.1000000000000000000001}}',
    );
    const exportConversation = vi.fn().mockResolvedValue({ format: "json", artifact });
    const client = { sessions: { export: exportConversation } } as unknown as FlameClient;
    installations.push(installConversationArchiveGateway(() => client, files));

    await exportConversationJson();

    expect(exportConversation).toHaveBeenCalledExactlyOnceWith("session-current", "json");
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-"),
      '{\n  "session": {\n    "id": "session-current"\n  },\n  "evidence": {\n    "count": 9007199254740993,\n    "fraction": 0.1000000000000000000001\n  }\n}',
      "application/json;charset=utf-8",
    );
  });

  it("downloads the complete Runtime evidence envelope with no UI reconstruction", async () => {
    const trajectory = {
      schemaVersion: 1,
      collectedAt: "2026-09-28T00:00:00Z",
      session: { id: "session-current" },
      runs: [{ id: "run_child" }],
      modelInvocations: [{ callId: "call_unknown", state: "unknown" }],
      feedback: [{ rating: "negative", text: "incorrect result" }],
      limitations: ["retained evidence"],
    };
    const exportTrajectory = vi.fn().mockResolvedValue({ trajectory });
    const client = { sessions: { exportTrajectory } } as unknown as FlameClient;
    installations.push(installConversationArchiveGateway(() => client, files));

    await exportSessionTrajectory();

    expect(exportTrajectory).toHaveBeenCalledExactlyOnceWith({ sessionId: "session-current" });
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-trajectory-"),
      JSON.stringify(trajectory, null, 2),
      "application/json;charset=utf-8",
    );
  });

  it("binds a Host owner to the exact client installed at composition time", async () => {
    const retiredExport = vi.fn().mockResolvedValue({ format: "md", markdown: "retired" });
    const successorExport = vi.fn().mockResolvedValue({ format: "md", markdown: "successor" });
    let runtimeClient = () => clientWithExport(retiredExport);
    installations.push(installConversationArchiveGateway(() => runtimeClient(), files));

    runtimeClient = () => clientWithExport(successorExport);
    await exportConversationMarkdown();

    expect(retiredExport).toHaveBeenCalledWith("session-current", "md");
    expect(successorExport).not.toHaveBeenCalled();
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-"),
      "retired",
      "text/markdown;charset=utf-8",
    );
  });

  it("retires an admitted export when the same Host observes a Runtime generation", async () => {
    const response = Promise.withResolvers<{ format: "md"; markdown: string }>();
    const exportConversation = vi.fn(() => response.promise);
    const successorExport = vi.fn().mockResolvedValue({ format: "md", markdown: "current" });
    let runtimeClient = () => clientWithExport(exportConversation);
    const installation = installConversationArchiveGateway(() => runtimeClient(), files);
    installations.push(installation);

    const retired = exportConversationMarkdown();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(exportConversation).toHaveBeenCalledOnce());
    runtimeClient = () => clientWithExport(successorExport);
    installation.replaceRuntimeGeneration();
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.resolve({ format: "md", markdown: "retired" });
    await retired;
    expect(settledAtReplacement).toBe(true);
    expect(mocks.download).not.toHaveBeenCalled();

    await exportConversationMarkdown();
    expect(successorExport).toHaveBeenCalledExactlyOnceWith("session-current", "md");
    expect(exportConversation).toHaveBeenCalledOnce();
    expect(mocks.download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-"),
      "current",
      "text/markdown;charset=utf-8",
    );
  });

  it("saves through the composed desktop host with only a suggested basename", async () => {
    const call = vi.fn(async (..._args: unknown[]) => true as unknown);
    const host = createDesktopHostClient({ call, on: async () => () => {} });
    const exportConversation = vi.fn().mockResolvedValue({ format: "md", markdown: "# Résumé" });
    installations.push(
      installConversationArchiveGateway(
        () => clientWithExport(exportConversation),
        createFileTransfer(host),
      ),
    );

    await exportConversationMarkdown();

    expect(call).toHaveBeenCalledExactlyOnceWith(
      "main.DesktopHost.SaveTextFile",
      expect.stringMatching(/^flame-session-current-[0-9T-]+\.md$/),
      "# Résumé",
    );
    expect(notifyError).not.toHaveBeenCalled();
  });

  it("reports a native save failure through the export failure notification", async () => {
    const call = vi.fn(async () => {
      throw new Error("write selected file: permission denied");
    });
    const host = createDesktopHostClient({ call, on: async () => () => {} });
    const exportConversation = vi.fn().mockResolvedValue({ format: "md", markdown: "# Title" });
    installations.push(
      installConversationArchiveGateway(
        () => clientWithExport(exportConversation),
        createFileTransfer(host),
      ),
    );

    await expect(exportConversationMarkdown()).resolves.toBeUndefined();

    expect(notifyError).toHaveBeenCalledExactlyOnceWith(
      expect.stringContaining("permission denied"),
      { source: "session" },
    );
  });
});

function clientWithExport(exportConversation: ReturnType<typeof vi.fn>): FlameClient {
  return {
    sessions: { export: exportConversation },
  } as unknown as FlameClient;
}

function observedSettlement(operation: Promise<unknown>): () => boolean {
  let settled = false;
  void operation.then(
    () => {
      settled = true;
    },
    () => {
      settled = true;
    },
  );
  return () => settled;
}

async function drainMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}
