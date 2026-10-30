import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-client";
import { stringifyJSON } from "@qimu/sdk/json";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { createConversationImportGateway } from "./runtimeConversationImportGateway";

vi.mock("@/features/connection/public/capabilities", () => ({ runtimeCapability: () => true }));
vi.mock("@/features/agent/public/session", () => ({
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
}));

let client: TestClient;
const owners: ReturnType<typeof createConversationImportGateway>[] = [];
beforeEach(() => {
  client = createTestClient({ start: false });
  Object.assign(client.agent.access, {
    selectAgentSession: vi.fn(),
    rehydrateSessionView: vi.fn().mockResolvedValue(undefined),
  });
});
afterEach(async () => {
  for (const owner of owners.splice(0)) owner.dispose();
  await client.dispose();
});

it("hands exact imported numeric evidence to the canonical Runtime admission", async () => {
  const source =
    '{"session":{"id":"imported"},"evidence":{"count":9007199254740993,"fraction":0.1000000000000000000001}}';
  const importSession = vi
    .fn()
    .mockResolvedValue({ session: { id: "imported", title: "Imported" } });
  const runtime = { sessions: { import: importSession } } as unknown as FlameClient;
  owners.push(
    createConversationImportGateway(client.workspace.access.conversationImport, () => runtime, {
      pickText: async () => source,
    }),
  );
  await client.workspace.access.conversationImport.importConversationJson();
  expect(importSession).toHaveBeenCalledOnce();
  expect(stringifyJSON(importSession.mock.calls[0]![0])).toBe(source);
  expect(client.agent.access.selectAgentSession).toHaveBeenCalledExactlyOnceWith("imported");
});

it("keeps its captured Runtime client until an explicit generation replacement", async () => {
  const first = vi.fn().mockResolvedValue({ session: { id: "first" } });
  const successor = vi.fn().mockResolvedValue({ session: { id: "successor" } });
  let current = { sessions: { import: first } } as unknown as FlameClient;
  const owner = createConversationImportGateway(
    client.workspace.access.conversationImport,
    () => current,
    {
      pickText: async () => '{"session":{"id":"captured"}}',
    },
  );
  owners.push(owner);
  current = { sessions: { import: successor } } as unknown as FlameClient;
  await client.workspace.access.conversationImport.importConversationJson();
  expect(first).toHaveBeenCalledOnce();
  expect(successor).not.toHaveBeenCalled();
  owner.replaceRuntimeGeneration();
  await client.workspace.access.conversationImport.importConversationJson();
  expect(first).toHaveBeenCalledOnce();
  expect(successor).toHaveBeenCalledOnce();
  expect(client.agent.access.selectAgentSession).toHaveBeenLastCalledWith("successor");
});
