import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkIndexActions } from "@/features/navigation/application/workIndexActions";
const mocks = vi.hoisted(() => ({
  activeSessionId: "",
  activeWorkspace: { status: "ready", cwd: undefined } as
    { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string },
  runtimeAvailable: true,
  open: vi.fn(),
  create: vi.fn(),
  focusComposer: vi.fn(),
  notifyError: vi.fn(),
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSessionId: () => mocks.activeSessionId,
  useActiveSessionWorkspace: () => mocks.activeWorkspace,
  useCreateSession: () => mocks.create,
  useDeleteSession: () => vi.fn(),
  useForkSession: () => vi.fn(),
  useRenameSession: () => vi.fn(),
  useToggleFavorite: () => vi.fn(),
}));
prepareTestClient(() => {
  Object.assign(testClient.agent.access, {
    selectAgentSession: vi.fn(),
    getActiveSessionId: () => mocks.activeSessionId,
  });
});
prepareTestClient(() => {
  Object.assign(testClient.agent.access, { createSession: () => mocks.create() });
});

vi.mock("@/features/composer/public/focus", () => ({
  focusComposer: mocks.focusComposer,
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => mocks.runtimeAvailable,
  useRuntimeCommandsAvailable: () => mocks.runtimeAvailable,
}));

vi.mock("@/features/notifications/public/notifications", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/notifications/public/notifications")>();
  return { ...actual, notifyError: mocks.notifyError };
});

let disposePicker = () => {};

beforeEach(() => {
  mocks.activeSessionId = "";
  mocks.activeWorkspace = { status: "ready", cwd: undefined };
  mocks.runtimeAvailable = true;
  mocks.open.mockReset();
  mocks.create.mockReset().mockResolvedValue("session-new");
  mocks.focusComposer.mockReset();
  mocks.notifyError.mockReset();
  disposePicker = stubInstance(testClient.directory.access, {
    open: mocks.open,
    browse: async () => undefined,
  });
});

afterEach(() => disposePicker());

describe("useWorkIndexActions directory selection", () => {
  it("opens workspace selection when creating the first Session", () => {
    const { result } = testClient.renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    expect(result.current.canCreateSession).toBe(true);
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.open).toHaveBeenCalledOnce();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("withdraws every Session mutation while Runtime commands are unavailable", () => {
    mocks.runtimeAvailable = false;
    const { result } = testClient.renderHook(() => useWorkIndexActions());

    act(() => {
      result.current.createSession();
      result.current.chooseSessionFolder();
      result.current.startSessionInFolder("/tmp/project");
    });

    expect(result.current.canCreateSession).toBe(false);
    expect(mocks.open).not.toHaveBeenCalled();
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("delegates the global new-session action to the active Session workspace owner", async () => {
    mocks.activeSessionId = "session-current";
    mocks.activeWorkspace = { status: "ready", cwd: "/tmp/current-project" };
    const { result } = testClient.renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith());
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });

  it("does not invent a default project while the active Session is resolving", () => {
    mocks.activeSessionId = "session-current";
    mocks.activeWorkspace = { status: "resolving", sessionId: "session-current" };
    const { result } = testClient.renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    expect(result.current.canCreateSession).toBe(false);
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("opens the runtime workspace selector for a different directory", () => {
    const { result } = testClient.renderHook(() => useWorkIndexActions());
    act(() => result.current.chooseSessionFolder());
    expect(mocks.open).toHaveBeenCalledOnce();
    expect(mocks.create).not.toHaveBeenCalled();
  });

  it("keeps focus in the current session when project creation is rejected", async () => {
    mocks.create.mockResolvedValue(null);
    const { result } = testClient.renderHook(() => useWorkIndexActions());

    act(() => result.current.startSessionInFolder("/tmp/project"));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ cwd: "/tmp/project" }));
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });
});
