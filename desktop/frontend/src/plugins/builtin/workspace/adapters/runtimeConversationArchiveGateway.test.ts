import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import {
  exportConversationMarkdown,
  exportSessionTrajectory,
} from "../application/conversationExport";
import { installConversationArchiveGateway } from "./runtimeConversationArchiveGateway";

const mocks = vi.hoisted(() => ({
  download: vi.fn<(filename: string, content: string, mime: string) => void>(),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => "session-current",
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
  rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  selectAgentSession: vi.fn(),
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  lookupExtensionByKey: vi.fn(),
  notifyError: vi.fn(),
}));

vi.mock("./browserFileTransfer", () => ({
  browserFileTransfer: () => ({ download: mocks.download, pickText: vi.fn() }),
}));

const installations: Array<ReturnType<typeof installConversationArchiveGateway>> = [];

beforeEach(() => mocks.download.mockReset());

afterEach(async () => {
  for (const installation of installations.splice(0).reverse()) installation.dispose();
  vi.restoreAllMocks();
});

describe("runtimeConversationArchiveGateway", () => {
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
    installations.push(installConversationArchiveGateway(() => client));

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
    installations.push(installConversationArchiveGateway(() => runtimeClient()));

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
    const installation = installConversationArchiveGateway(() => runtimeClient());
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
