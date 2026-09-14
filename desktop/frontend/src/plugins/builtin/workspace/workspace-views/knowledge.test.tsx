import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { WORKSPACE_KNOWLEDGE_KEY } from "../application/workspaceQueries";
import { queryClient } from "@/lib/queryClient";
import { KnowledgeOwner } from "../application/knowledge";
import {
  WorkspaceKnowledgeRevisionConflictError,
  type WorkspaceKnowledgeGateway,
} from "../application/ports/knowledgeGateway";
import { KnowledgeTab } from "./knowledge";

const model = vi.hoisted(() => ({ notifyError: vi.fn() }));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  notifyError: model.notifyError,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/work/alpha" }),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: () => true,
}));

let owner: KnowledgeOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  queryClient.clear();
  vi.clearAllMocks();
});

it("shows the conflict reason and preserves the draft for an explicit retry against the latest revision", async () => {
  const reason = "The knowledge document changed after it was read.";
  let document = { path: "/work/alpha/FLAME.md", content: "Original", revision: "rev-1" };
  await loadPluginsForTest(
    definePlugin({
      name: "test.knowledge",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_KNOWLEDGE_KEY,
          fetcher: async () => [{ scope: "cwd", ...document }],
        });
      },
    }),
  );
  const read = vi.fn<WorkspaceKnowledgeGateway["read"]>().mockResolvedValue({
    path: "/work/alpha/FLAME.md",
    content: "External edit",
    revision: "rev-2",
  });
  const save = vi
    .fn<WorkspaceKnowledgeGateway["save"]>()
    .mockImplementationOnce(async () => {
      document = { ...document, content: "External edit", revision: "rev-2" };
      throw new WorkspaceKnowledgeRevisionConflictError(new Error(reason));
    })
    .mockImplementationOnce(async () => {
      document = { ...document, content: "My draft", revision: "rev-3" };
      return document;
    });
  owner = KnowledgeOwner.install({ read, save });
  render(
    <QueryClientProvider client={queryClient}>
      <KnowledgeTab />
    </QueryClientProvider>,
  );

  fireEvent.click(await screen.findByRole("button", { name: /\/work\/alpha\/FLAME.md/ }));
  const editor = screen.getByRole("textbox", {
    name: "Knowledge content for /work/alpha/FLAME.md",
  }) as HTMLTextAreaElement;
  fireEvent.change(editor, { target: { value: "My draft" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() =>
    expect(model.notifyError).toHaveBeenCalledWith("Knowledge save failed", {
      description: reason,
      source: "knowledge",
    }),
  );
  expect(editor.value).toBe("My draft");
  expect(save).toHaveBeenCalledExactlyOnceWith({
    scope: "cwd",
    cwd: "/work/alpha",
    content: "My draft",
    expectedRevision: "rev-1",
  });
  expect(read).toHaveBeenCalledExactlyOnceWith({ scope: "cwd", cwd: "/work/alpha" });

  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(save).toHaveBeenLastCalledWith({
      scope: "cwd",
      cwd: "/work/alpha",
      content: "My draft",
      expectedRevision: "rev-2",
    }),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(true),
  );
  expect(editor.value).toBe("My draft");
});

it("reports a failed conflict reload without losing the draft or advancing its revision", async () => {
  const conflict = "The knowledge document changed after it was read.";
  await loadPluginsForTest(
    definePlugin({
      name: "test.knowledge",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_KNOWLEDGE_KEY,
          fetcher: async () => [
            { scope: "cwd", path: "/work/alpha/FLAME.md", content: "Original", revision: "rev-1" },
          ],
        });
      },
    }),
  );
  const read = vi
    .fn<WorkspaceKnowledgeGateway["read"]>()
    .mockRejectedValue(new Error("Connection closed"));
  const save = vi
    .fn<WorkspaceKnowledgeGateway["save"]>()
    .mockRejectedValue(new WorkspaceKnowledgeRevisionConflictError(new Error(conflict)));
  owner = KnowledgeOwner.install({ read, save });
  render(
    <QueryClientProvider client={queryClient}>
      <KnowledgeTab />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: /\/work\/alpha\/FLAME.md/ }));
  const editor = screen.getByRole("textbox", {
    name: "Knowledge content for /work/alpha/FLAME.md",
  }) as HTMLTextAreaElement;
  fireEvent.change(editor, { target: { value: "My draft" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() =>
    expect(model.notifyError).toHaveBeenCalledExactlyOnceWith("Knowledge save failed", {
      description: `${conflict}\nCould not reload the latest document: Connection closed`,
      source: "knowledge",
    }),
  );
  expect(editor.value).toBe("My draft");
  expect(save).toHaveBeenCalledOnce();
  expect(read).toHaveBeenCalledOnce();

  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(model.notifyError).toHaveBeenCalledTimes(2));
  expect(save).toHaveBeenNthCalledWith(2, {
    scope: "cwd",
    cwd: "/work/alpha",
    content: "My draft",
    expectedRevision: "rev-1",
  });
  expect(editor.value).toBe("My draft");
});
