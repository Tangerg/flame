import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type ConversationImportOwner } from "@/features/workspace/application/conversationImport";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;

import { type ConversationImportGateway } from "@/features/workspace/application/ports/conversationImportGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";
import { pickTextFile } from "@/features/workspace/adapters/fileTransfer";
const mocks = vi.hoisted(() => ({
  sessionExport: true,
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
  invalidateAgentSessions: mocks.invalidateAgentSessions,
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
let files: FileTransferPort | null;

beforeEach(() => {
  testClient = createTestClient({ start: false });
  Object.assign(testClient.agent.access, {
    selectAgentSession: mocks.selectAgentSession,
  });
  Object.assign(testClient.agent.access, { rehydrateSessionView: mocks.rehydrateSessionView });
  mocks.sessionExport = true;
  mocks.invalidateAgentSessions.mockReset().mockResolvedValue(undefined);
  mocks.notifyError.mockReset();
  mocks.rehydrateSessionView.mockReset().mockResolvedValue(undefined);
  mocks.selectAgentSession.mockReset();
  mocks.success.mockReset();
  files = null;
});

afterEach(async () => {
  for (const dispose of disposers.splice(0).reverse()) dispose();
  await testClient.dispose();
  vi.restoreAllMocks();
});

describe("conversation import generation", () => {
  it("hands Runtime the artifact it exported for admission", async () => {
    const importConversation = vi.fn().mockResolvedValue({ id: "imported", title: "Imported" });
    const artifact = { version: 30, session: { id: "imported" }, runs: [], items: [] };
    installFiles({ pickText: () => Promise.resolve(JSON.stringify(artifact)) });
    installGateway({ importConversation });

    await testClient.workspace.access.conversationImport.importConversationJson();

    expect(importConversation).toHaveBeenCalledExactlyOnceWith(artifact);
    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(mocks.selectAgentSession).toHaveBeenCalledWith("imported");
  });

  it("retires a picker before its continuation can borrow the successor gateway", async () => {
    const picker = Promise.withResolvers<string | null>();
    const successorImport = vi.fn().mockResolvedValue({ id: "successor-session" });
    installFiles({ pickText: () => picker.promise });
    installGateway({ importConversation: vi.fn() });

    const retired = testClient.workspace.access.conversationImport.importConversationJson();
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
      pickText: () => Promise.resolve(validArtifact("old-session")),
    });
    installGateway({ importConversation: retiredImport });

    const retired = testClient.workspace.access.conversationImport.importConversationJson();
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

  it("retires the import while rehydrate is pending before query repair or navigation", async () => {
    const hydration = Promise.withResolvers<void>();
    mocks.rehydrateSessionView.mockReturnValueOnce(hydration.promise);
    installFiles({
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    const gateway = {
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    } as unknown as ConversationImportGateway;
    const owner = installGateway(gateway);

    const retired = testClient.workspace.access.conversationImport.importConversationJson();
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
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    const gateway = {
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    } as unknown as ConversationImportGateway;
    const owner = installGateway(gateway);

    const retired = testClient.workspace.access.conversationImport.importConversationJson();
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
      pickText: () => Promise.resolve(validArtifact("imported")),
    });
    installGateway({
      importConversation: vi.fn().mockResolvedValue({ id: "imported", title: "Imported" }),
    });

    await testClient.workspace.access.conversationImport.importConversationJson();

    expect(mocks.rehydrateSessionView).toHaveBeenCalledWith("imported");
    expect(mocks.selectAgentSession).toHaveBeenCalledWith("imported");
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.notifyError).not.toHaveBeenCalled();
  });

  it("reports an unreadable selection and admits the next explicit import", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    vi.spyOn(console, "error").mockImplementation(() => {});
    const importConversation = vi
      .fn<ConversationImportGateway["importConversation"]>()
      .mockResolvedValue({ id: "imported", title: "Imported" });
    const unreadable = new File([validArtifact("unreadable")], "unreadable.json");
    vi.spyOn(unreadable, "text").mockRejectedValue(
      new DOMException("The I/O read operation failed.", "NotReadableError"),
    );
    installFiles({ pickText: pickTextFile });
    installGateway({ importConversation });

    const failed = testClient.workspace.access.conversationImport.importConversationJson();
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
    const retried = testClient.workspace.access.conversationImport.importConversationJson();
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
    const importConversation = vi.fn<ConversationImportGateway["importConversation"]>();
    installFiles({ pickText: pickTextFile });
    installGateway({ importConversation });

    const cancelled = testClient.workspace.access.conversationImport.importConversationJson();
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
    const retiredImport = vi.fn<ConversationImportGateway["importConversation"]>();
    const successorImport = vi
      .fn<ConversationImportGateway["importConversation"]>()
      .mockResolvedValue({ id: "successor-session", title: "Successor" });
    installFiles({ pickText: pickTextFile });
    const owner = installGateway({ importConversation: retiredImport });

    const retired = testClient.workspace.access.conversationImport.importConversationJson();
    await vi.waitFor(() => expect(click).toHaveBeenCalledOnce());
    choose(click.mock.instances[0] as HTMLInputElement, slow);
    owner.replaceRuntimeGeneration(() => ({ importConversation: successorImport }));
    await retired;
    read.reject(new DOMException("The I/O read operation failed.", "NotReadableError"));
    await drainMicrotasks();

    expect(mocks.notifyError).not.toHaveBeenCalled();
    expect(retiredImport).not.toHaveBeenCalled();

    const artifact = validArtifact("successor-session");
    const current = testClient.workspace.access.conversationImport.importConversationJson();
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
    installFiles({ pickText });
    installGateway({ importConversation });

    const first = testClient.workspace.access.conversationImport.importConversationJson();
    const repeated = testClient.workspace.access.conversationImport.importConversationJson();

    expect(repeated).toBe(first);
    expect(pickText).toHaveBeenCalledOnce();
    picker.resolve(null);
    await Promise.all([first, repeated]);
    expect(importConversation).not.toHaveBeenCalled();
  });
});

function installGateway(gateway: ConversationImportGateway): ConversationImportOwner {
  if (!files) throw new Error("test file transfer is not installed");
  const owner = testClient.workspace.access.conversationImport.install({
    gateway,
    files,
  });
  disposers.push(() => owner.dispose());
  return owner;
}

function installFiles(next: FileTransferPort): void {
  files = next;
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
