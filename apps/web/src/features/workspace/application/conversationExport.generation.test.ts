import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ConversationArchiveOwner,
  exportConversationMarkdown,
  exportConversationJson,
  exportSessionTrajectory,
  importConversationJson,
} from "@/features/workspace/application/conversationExport";
import { type ConversationArchiveGateway } from "@/features/workspace/application/ports/conversationArchiveGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
import { createFileTransfer } from "@/features/workspace/adapters/fileTransfer";
const mocks = vi.hoisted(() => ({
  sessionExport: true,
  activeSessionId: "session-current" as string | undefined,
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
  notifyError: vi.fn(),
  rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  selectAgentSession: vi.fn(),
  success: vi.fn(),
}));

vi.mock("@/features/connection/public/capabilities", () => ({
  runtimeCapability: () => mocks.sessionExport,
}));

vi.mock("@/features/agent/public/session", () => ({
  getActiveSessionId: () => mocks.activeSessionId,
  invalidateAgentSessions: mocks.invalidateAgentSessions,
  rehydrateSessionView: mocks.rehydrateSessionView,
  selectAgentSession: mocks.selectAgentSession,
}));

vi.mock("@/extensions/selectors/extensions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/selectors/extensions")>();
  return { ...actual, lookupExtensionByKey: vi.fn() };
});
vi.mock("@/features/notifications/public/notifications", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/notifications/public/notifications")>();
  return { ...actual, notifyError: mocks.notifyError };
});

vi.mock("sonner", () => ({ toast: { success: mocks.success } }));

const disposers: Array<() => void> = [];

let download = vi.fn<(filename: string, content: string, mime: string) => Promise<void>>();

let files: FileTransferPort | null;

beforeEach(() => {
  mocks.activeSessionId = "session-current";
  mocks.sessionExport = true;
  mocks.invalidateAgentSessions.mockReset().mockResolvedValue(undefined);
  mocks.notifyError.mockReset();
  mocks.rehydrateSessionView.mockReset().mockResolvedValue(undefined);
  mocks.selectAgentSession.mockReset();
  mocks.success.mockReset();
  download = vi
    .fn<(filename: string, content: string, mime: string) => Promise<void>>()
    .mockResolvedValue(undefined);
  files = null;
});

afterEach(() => {
  for (const dispose of disposers.splice(0).reverse()) dispose();
  vi.restoreAllMocks();
});

describe("conversation archive generation", () => {
  it.each([
    ["Markdown", exportConversationMarkdown],
    ["JSON", exportConversationJson],
    ["trajectory", exportSessionTrajectory],
  ] as const)("waits for the %s file handoff to finish", async (_format, run) => {
    const handoff = Promise.withResolvers<void>();
    download.mockImplementationOnce(() => handoff.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({
      exportConversation: vi.fn().mockResolvedValue("captured content"),
      exportTrajectory: vi.fn().mockResolvedValue("captured evidence"),
    });

    const exporting = run();
    const settled = observedSettlement(exporting);
    try {
      await vi.waitFor(() => expect(download).toHaveBeenCalledOnce());
      expect(settled()).toBe(false);
      expect(mocks.notifyError).not.toHaveBeenCalled();
    } finally {
      handoff.resolve();
      await exporting;
    }
    expect(settled()).toBe(true);
  });

  it.each([
    ["Markdown", exportConversationMarkdown],
    ["JSON", exportConversationJson],
    ["trajectory", exportSessionTrajectory],
  ] as const)("reports a deferred %s save failure", async (_format, run) => {
    const handoff = Promise.withResolvers<void>();
    const failure = new Error("selected file could not be written");
    download.mockImplementationOnce(() => handoff.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({
      exportConversation: vi.fn().mockResolvedValue("captured content"),
      exportTrajectory: vi.fn().mockResolvedValue("captured evidence"),
    });

    const exporting = run();
    await vi.waitFor(() => expect(download).toHaveBeenCalledOnce());
    handoff.reject(failure);
    await exporting;
    expect(mocks.notifyError).toHaveBeenCalledExactlyOnceWith(failure.message, {
      source: "session",
    });
  });

  it("retires pending file feedback without changing the admitted material", async () => {
    const handoff = Promise.withResolvers<void>();
    download.mockImplementationOnce(() => handoff.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    const owner = installGateway({
      exportTrajectory: vi.fn().mockResolvedValue("original captured evidence"),
    });

    const exporting = exportSessionTrajectory();
    await vi.waitFor(() => expect(download).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration(() => ({
      exportConversation: async () => "current conversation",
      exportTrajectory: async () => "current evidence",
      importConversation: async () => ({ id: "current-import" }),
    }));
    await exporting;
    handoff.reject(new Error("late native write failure"));
    await drainMicrotasks();
    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(download.mock.calls[0]?.[1]).toBe("original captured evidence");

    await exportSessionTrajectory();
    expect(download).toHaveBeenLastCalledWith(
      expect.stringContaining("flame-session-current-trajectory-"),
      "current evidence",
      "application/json;charset=utf-8",
    );
  });

  it("exports captured Session evidence without reading the visible conversation", async () => {
    const response = Promise.withResolvers<string>();
    const exportTrajectory = vi.fn(() => response.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportTrajectory });

    const exporting = exportSessionTrajectory();
    mocks.activeSessionId = "session-other";
    response.resolve('{"schemaVersion":1,"evidence":"complete"}');
    await exporting;

    expect(exportTrajectory).toHaveBeenCalledExactlyOnceWith("session-current");
    expect(download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-trajectory-"),
      '{"schemaVersion":1,"evidence":"complete"}',
      "application/json;charset=utf-8",
    );
  });

  it("does not request evidence without the Runtime export capability", async () => {
    const exportTrajectory = vi.fn();
    mocks.sessionExport = false;
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportTrajectory });
    await exportSessionTrajectory();
    expect(exportTrajectory).not.toHaveBeenCalled();
    expect(download).not.toHaveBeenCalled();
    expect(mocks.notifyError).toHaveBeenCalledOnce();
  });

  it("reports evidence export failure without downloading a partial substitute", async () => {
    const failure = new Error("session is busy");
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportTrajectory: vi.fn().mockRejectedValue(failure) });

    await exportSessionTrajectory();

    expect(mocks.notifyError).toHaveBeenCalledExactlyOnceWith("session is busy", {
      source: "session",
    });
    expect(download).not.toHaveBeenCalled();
  });

  it("reports conversation export failure without downloading a client-made substitute", async () => {
    const failure = new Error("session is busy");
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportConversation: vi.fn().mockRejectedValue(failure) });

    await exportConversationMarkdown();

    expect(mocks.notifyError).toHaveBeenCalledExactlyOnceWith("session is busy", {
      source: "session",
    });
    expect(download).not.toHaveBeenCalled();
  });

  it("hands Runtime the artifact it exported for admission", async () => {
    const importConversation = vi.fn().mockResolvedValue({ id: "imported", title: "Imported" });
    const artifact = { version: 30, session: { id: "imported" }, runs: [], items: [] };
    installFiles({ saveText: download, pickText: () => Promise.resolve(JSON.stringify(artifact)) });
    installGateway({ importConversation });

    await importConversationJson();

    expect(importConversation).toHaveBeenCalledExactlyOnceWith(artifact);
    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).toHaveBeenCalledWith("imported");
  });

  it("retires a pending evidence export before the previous Runtime can download", async () => {
    const response = Promise.withResolvers<string>();
    const exportTrajectory = vi.fn(() => response.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    const owner = installGateway({ exportTrajectory });

    const exporting = exportSessionTrajectory();
    await vi.waitFor(() => expect(exportTrajectory).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration(
      () =>
        ({
          exportTrajectory: vi.fn().mockResolvedValue("current"),
        }) as unknown as ConversationArchiveGateway,
    );
    await exporting;
    response.resolve("retired");
    await drainMicrotasks();

    expect(download).not.toHaveBeenCalled();
    await exportSessionTrajectory();
    expect(download).toHaveBeenCalledWith(
      expect.stringContaining("flame-session-current-trajectory-"),
      "current",
      "application/json;charset=utf-8",
    );
  });

  it("does not request evidence when no Session is selected", async () => {
    const exportTrajectory = vi.fn();
    mocks.activeSessionId = undefined;
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportTrajectory });

    await exportSessionTrajectory();

    expect(exportTrajectory).not.toHaveBeenCalled();
    expect(download).not.toHaveBeenCalled();
  });

  it("retires a picker before its continuation can borrow the successor gateway", async () => {
    const picker = Promise.withResolvers<string | null>();
    const successorImport = vi.fn().mockResolvedValue({ id: "successor-session" });
    installFiles({ saveText: download, pickText: () => picker.promise });
    installGateway({ importConversation: vi.fn() });

    const retired = importConversationJson();
    await drainMicrotasks();
    installGateway({ importConversation: successorImport });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    picker.resolve(validArtifact("old-session"));
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(successorImport).not.toHaveBeenCalled();
    expect(mocks.rehydrateSessionView).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("settles an in-flight import at replacement and rejects every late product effect", async () => {
    const response = Promise.withResolvers<{ id: string; title?: string }>();
    const retiredImport = vi.fn(() => response.promise);
    installFiles({
      saveText: download,
      pickText: () => Promise.resolve(validArtifact("old-session")),
    });
    installGateway({ importConversation: retiredImport });

    const retired = importConversationJson();
    await vi.waitFor(() => expect(retiredImport).toHaveBeenCalledOnce());
    installGateway({ importConversation: vi.fn().mockResolvedValue({ id: "successor-session" }) });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.resolve({ id: "old-session", title: "Old" });
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.rehydrateSessionView).not.toHaveBeenCalled();
    expect(mocks.invalidateAgentSessions).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("does not turn a retired export failure into a successor download", async () => {
    const response = Promise.withResolvers<never>();
    const retiredExport = vi.fn(() => response.promise);
    installFiles({ saveText: download, pickText: vi.fn() });
    installGateway({ exportConversation: retiredExport });

    const retired = exportConversationMarkdown();
    await vi.waitFor(() => expect(retiredExport).toHaveBeenCalledOnce());
    installGateway({
      exportConversation: vi.fn().mockResolvedValue("successor"),
    });
    const hasSettled = observedSettlement(retired);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    response.reject(new Error("old connection closed"));
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(download).not.toHaveBeenCalled();
  });

  it("retires the import while rehydrate is pending before query repair or navigation", async () => {
    const hydration = Promise.withResolvers<void>();
    mocks.rehydrateSessionView.mockReturnValueOnce(hydration.promise);
    installFiles({
      saveText: download,
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    const gateway = {
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    } as unknown as ConversationArchiveGateway;
    const owner = installGateway(gateway);

    const retired = importConversationJson();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(mocks.rehydrateSessionView).toHaveBeenCalledWith("imported"));
    owner.replaceRuntimeGeneration(() => gateway);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    hydration.resolve();
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.invalidateAgentSessions).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("retires query repair before it can publish old import navigation", async () => {
    const repair = Promise.withResolvers<void>();
    mocks.invalidateAgentSessions.mockReturnValueOnce(repair.promise);
    installFiles({
      saveText: download,
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    const gateway = {
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    } as unknown as ConversationArchiveGateway;
    const owner = installGateway(gateway);

    const retired = importConversationJson();
    const hasSettled = observedSettlement(retired);
    await vi.waitFor(() => expect(mocks.invalidateAgentSessions).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration(() => gateway);
    await drainMicrotasks();
    const settledAtReplacement = hasSettled();

    repair.resolve();
    await retired;

    expect(settledAtReplacement).toBe(true);
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("publishes an accepted import even when Session collection repair fails", async () => {
    mocks.invalidateAgentSessions.mockRejectedValueOnce(new Error("query unavailable"));
    installFiles({
      saveText: download,
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    installGateway({
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    });

    await importConversationJson();

    expect(mocks.rehydrateSessionView).toHaveBeenCalledWith("imported");
    expect(mocks.selectAgentSession).toHaveBeenCalledWith("imported");
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.notifyError).not.toHaveBeenCalled();
  });

  it("reports an unreadable selection and admits the next explicit import", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    vi.spyOn(console, "error").mockImplementation(() => {});
    const importConversation = vi
      .fn<ConversationArchiveGateway["importConversation"]>()
      .mockResolvedValue({ id: "imported", title: "Imported" });
    const unreadable = new File([validArtifact("unreadable")], "unreadable.json");
    vi.spyOn(unreadable, "text").mockRejectedValue(
      new DOMException("The I/O read operation failed.", "NotReadableError"),
    );
    installFiles(createFileTransfer({ saveTextFile: vi.fn().mockResolvedValue(true) }));
    installGateway(completeGateway(importConversation));

    const failed = importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledOnce());
    choose(click.mock.instances[0] as HTMLInputElement, unreadable);
    await failed;

    expect(mocks.notifyError).toHaveBeenCalledExactlyOnceWith(
      "Couldn't read the selected file. Select it again or choose another JSON file.",
      { source: "import" },
    );
    expect(importConversation).not.toHaveBeenCalled();
    expect(mocks.rehydrateSessionView).not.toHaveBeenCalled();
    expect(mocks.invalidateAgentSessions).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();

    const artifact = validArtifact("imported");
    const retried = importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(2));
    choose(click.mock.instances[1] as HTMLInputElement, new File([artifact], "readable.json"));
    await retried;

    expect(importConversation).toHaveBeenCalledExactlyOnceWith(JSON.parse(artifact));
    expect(mocks.selectAgentSession).toHaveBeenCalledExactlyOnceWith("imported");
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.notifyError).toHaveBeenCalledOnce();
  });

  it("keeps a cancelled selection quiet", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    const importConversation = vi.fn<ConversationArchiveGateway["importConversation"]>();
    installFiles(createFileTransfer({ saveTextFile: vi.fn().mockResolvedValue(true) }));
    installGateway(completeGateway(importConversation));

    const cancelled = importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledOnce());
    (click.mock.instances[0] as HTMLInputElement).dispatchEvent(new Event("cancel"));
    await cancelled;

    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(importConversation).not.toHaveBeenCalled();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it("keeps a retired read failure quiet without blocking the successor import", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    const read = Promise.withResolvers<string>();
    const slow = new File([validArtifact("retired")], "slow.json");
    vi.spyOn(slow, "text").mockReturnValue(read.promise);
    const retiredImport = vi.fn<ConversationArchiveGateway["importConversation"]>();
    const successorImport = vi
      .fn<ConversationArchiveGateway["importConversation"]>()
      .mockResolvedValue({ id: "successor-session", title: "Successor" });
    installFiles(createFileTransfer({ saveTextFile: vi.fn().mockResolvedValue(true) }));
    const owner = installGateway(completeGateway(retiredImport));

    const retired = importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledOnce());
    choose(click.mock.instances[0] as HTMLInputElement, slow);
    owner.replaceRuntimeGeneration(() => completeGateway(successorImport));
    await retired;
    read.reject(new DOMException("The I/O read operation failed.", "NotReadableError"));
    await drainMicrotasks();

    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(retiredImport).not.toHaveBeenCalled();

    const artifact = validArtifact("successor-session");
    const current = importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(2));
    choose(click.mock.instances[1] as HTMLInputElement, new File([artifact], "current.json"));
    await current;

    expect(successorImport).toHaveBeenCalledExactlyOnceWith(JSON.parse(artifact));
    expect(mocks.selectAgentSession).toHaveBeenCalledExactlyOnceWith("successor-session");
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.notifyError).not.toHaveBeenCalled();
  });

  it("owns repeated import intent as one picker and one mutation", async () => {
    const picker = Promise.withResolvers<string | null>();
    const importConversation = vi.fn().mockResolvedValue({ id: "imported" });
    const pickText = vi.fn(() => picker.promise);
    installFiles({ saveText: download, pickText });
    installGateway({ importConversation });

    const first = importConversationJson();
    const repeated = importConversationJson();

    expect(repeated).toBe(first);
    expect(pickText).toHaveBeenCalledOnce();
    picker.resolve(null);
    await Promise.all([first, repeated]);
    expect(importConversation).not.toHaveBeenCalled();
  });
});

function installGateway(gateway: Partial<ConversationArchiveGateway>): ConversationArchiveOwner {
  if (!files) throw new Error("test file transfer is not installed");
  const owner = ConversationArchiveOwner.install({
    gateway: gateway as ConversationArchiveGateway,
    files,
  });
  disposers.push(() => owner.dispose());
  return owner;
}

function installFiles(next: FileTransferPort): void {
  files = next;
}

function completeGateway(
  importConversation: ConversationArchiveGateway["importConversation"],
): ConversationArchiveGateway {
  return {
    exportConversation: vi.fn().mockResolvedValue("unused conversation"),
    exportTrajectory: vi.fn().mockResolvedValue("unused evidence"),
    importConversation,
  };
}

function choose(picker: HTMLInputElement, file: File): void {
  Object.defineProperty(picker, "files", { configurable: true, value: [file] });
  picker.dispatchEvent(new Event("change"));
}

function validArtifact(id: string): string {
  return JSON.stringify({ version: 30, session: { id }, runs: [], items: [] });
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
