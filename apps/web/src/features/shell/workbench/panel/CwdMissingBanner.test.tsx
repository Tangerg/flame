import { act, fireEvent, screen } from "@testing-library/react";
import { beforeEach, onTestFinished, expect, it, vi } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { CwdMissingBanner } from "./CwdMissingBanner";

const source = vi.hoisted(() => ({
  session: {
    id: "missing-1",
    revision: 1,
    workspace: { path: "/missing", availability: "missing" },
  },
  relocate: vi.fn<(...args: unknown[]) => Promise<boolean>>(),
}));
vi.mock("@/features/agent/public/session", async (original) => ({
  ...(await original<typeof import("@/features/agent/public/session")>()),
  useActiveSession: () => source.session,
  useRelocateSession: () => source.relocate,
}));
vi.mock("@/features/connection/public/capabilities", async (original) => ({
  ...(await original<typeof import("@/features/connection/public/capabilities")>()),
  useRuntimeCapability: () => true,
}));
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
  source.session = {
    id: "missing-1",
    revision: 1,
    workspace: { path: "/missing", availability: "missing" },
  };
  source.relocate.mockReset().mockResolvedValue(false);
});

it("keeps the relocation target's opened revision across Session refreshes", async () => {
  const view = client.render(<CwdMissingBanner />);
  fireEvent.click(screen.getByRole("button", { name: /relocate/i }));
  const input = screen.getByRole<HTMLInputElement>("textbox");
  fireEvent.change(input, { target: { value: "/replacement" } });
  source.session = { ...source.session, revision: 2 };
  view.rerender(<CwdMissingBanner />);
  await act(async () => {
    fireEvent.keyDown(input, { key: "Enter" });
  });
  expect(source.relocate).toHaveBeenCalledExactlyOnceWith("missing-1", 1, "/replacement");
  expect(screen.getByRole<HTMLInputElement>("textbox").value).toBe("/replacement");
});

it("does not rebind an old path draft or late success to the next Session", async () => {
  const pending = Promise.withResolvers<boolean>();
  source.relocate.mockReturnValueOnce(pending.promise);
  const view = client.render(<CwdMissingBanner />);
  fireEvent.click(screen.getByRole("button", { name: /relocate/i }));
  const input = screen.getByRole<HTMLInputElement>("textbox");
  fireEvent.change(input, { target: { value: "/first-draft" } });
  await act(async () => {
    fireEvent.keyDown(input, { key: "Enter" });
  });
  source.session = {
    id: "missing-2",
    revision: 7,
    workspace: { path: "/second-missing", availability: "missing" },
  };
  view.rerender(<CwdMissingBanner />);
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /relocate/i }));
  const next = screen.getByRole<HTMLInputElement>("textbox");
  expect(next.value).toBe("/second-missing");
  await act(async () => {
    pending.resolve(true);
  });
  expect(screen.getByRole<HTMLInputElement>("textbox").value).toBe("/second-missing");
  fireEvent.change(next, { target: { value: "/second-draft" } });
  expect(next.value).toBe("/second-draft");
  expect(source.relocate).toHaveBeenCalledExactlyOnceWith("missing-1", 1, "/first-draft");
});
