import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import { PackageActions } from "./PackageActions";
import type { packageOperations } from "../application/packages";

const projection = vi.hoisted(() => ({
  session: { id: "ses_original", revision: 7, title: "Original" },
  invalidate: vi.fn(),
}));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSession: () => projection.session,
  invalidateAgentSessions: projection.invalidate,
}));

afterEach(() => {
  cleanup();
  projection.session = { id: "ses_original", revision: 7, title: "Original" };
  vi.clearAllMocks();
});

function fixture(
  renameSession = vi.fn(async () => ({ id: "ses_original", revision: 8, title: "Reviewed" })),
) {
  const lifetime = new AbortController();
  const installation: PluginInstallation = {
    id: "940ac827-b431-455b-af4b-e3a170bcfda0",
    source: "/package",
    state: "enabled",
    presentation: "admitted",
    realization: { type: "available" },
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    selected: {
      name: "Actions",
      digest: "1".repeat(64),
      actions: [{ id: "rename", title: "Name investigation", operation: "renameSession" }],
      servers: [],
      inputs: [],
      themes: [],
      views: [],
      skills: [],
      diagnostics: [],
    },
  };
  const operations = { signal: lifetime.signal, renameSession } as unknown as ReturnType<
    typeof packageOperations.get
  >;
  const view = render(<PackageActions installation={installation} operations={operations} />);
  return { view, installation, operations, lifetime, renameSession };
}

it("submits only explicit intent and retains the Session revision reviewed when opened", async () => {
  const f = fixture();
  expect(f.renameSession).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Name investigation · Rename" }));
  expect(f.renameSession).not.toHaveBeenCalled();
  fireEvent.change(screen.getByRole("textbox", { name: "Session title" }), {
    target: { value: "Reviewed" },
  });
  projection.session = { id: "ses_successor", revision: 12, title: "Successor" };
  f.view.rerender(<PackageActions installation={f.installation} operations={f.operations} />);
  expect(screen.getByText("Rename Session ses_original · revision 7")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Rename" }));
  await waitFor(() =>
    expect(f.renameSession).toHaveBeenCalledExactlyOnceWith({
      installationId: f.installation.id,
      digest: "1".repeat(64),
      actionId: "rename",
      update: { sessionId: "ses_original", expectedRevision: 7, title: "Reviewed" },
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(projection.invalidate).toHaveBeenCalledOnce();
  expect(screen.getByText("Renamed ses_original to “Reviewed”.")).toBeTruthy();
});

it("preserves a refused draft without adopting a newer Session revision", async () => {
  const rename = vi.fn(async () => {
    throw new Error("revision conflict");
  });
  const f = fixture(rename);
  fireEvent.click(screen.getByRole("button", { name: "Name investigation · Rename" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Session title" }), {
    target: { value: "Reviewed" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Rename" }));
  expect((await screen.findByRole("alert")).textContent).toContain("revision conflict");
  expect((screen.getByRole("textbox", { name: "Session title" }) as HTMLInputElement).value).toBe(
    "Reviewed",
  );
  projection.session.revision = 8;
  f.view.rerender(<PackageActions installation={f.installation} operations={f.operations} />);
  expect(screen.getByText("Rename Session ses_original · revision 7")).toBeTruthy();
  expect(rename).toHaveBeenCalledOnce();
  expect(projection.invalidate).not.toHaveBeenCalled();
});

it("closing a form leaves an accepted command alone and prevents its result closing a successor form", async () => {
  let settle!: () => void;
  const rename = vi.fn(
    () =>
      new Promise<{ id: string; revision: number; title: string }>((resolve) => {
        settle = () => resolve({ id: "ses_original", revision: 8, title: "Original" });
      }),
  );
  fixture(rename);
  fireEvent.click(screen.getByRole("button", { name: "Name investigation · Rename" }));
  fireEvent.click(screen.getByRole("button", { name: "Rename" }));
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  fireEvent.click(screen.getByRole("button", { name: "Name investigation · Rename" }));
  await act(async () => settle());
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(rename).toHaveBeenCalledOnce();
});

it("retires a draft on connection replacement without invoking the successor", () => {
  const f = fixture();
  fireEvent.click(screen.getByRole("button", { name: "Name investigation · Rename" }));
  act(() => f.lifetime.abort());
  const successor = {
    ...f.operations,
    signal: new AbortController().signal,
    renameSession: vi.fn(),
  };
  f.view.rerender(<PackageActions installation={f.installation} operations={successor} />);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(f.renameSession).not.toHaveBeenCalled();
  expect(successor.renameSession).not.toHaveBeenCalled();
});
