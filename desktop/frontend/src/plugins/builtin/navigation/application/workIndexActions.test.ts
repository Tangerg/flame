import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { configureWorkingDirectoryPicker } from "./ports/workingDirectoryPicker";
import { useWorkIndexActions } from "./workIndexActions";

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

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  selectAgentSession: vi.fn(),
  getActiveSessionId: () => mocks.activeSessionId,
  createSession: mocks.create,
  useActiveSessionId: () => mocks.activeSessionId,
  useActiveSessionWorkspace: () => mocks.activeWorkspace,
  useCreateSession: () => mocks.create,
  useDeleteSession: () => vi.fn(),
  useForkSession: () => vi.fn(),
  useRenameSession: () => vi.fn(),
  useToggleFavorite: () => vi.fn(),
}));

vi.mock("@/plugins/builtin/chat/composer/public/focus", () => ({
  focusComposer: mocks.focusComposer,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => mocks.runtimeAvailable,
  useRuntimeCommandsAvailable: () => mocks.runtimeAvailable,
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  notifyError: mocks.notifyError,
}));

let disposePicker = () => {};

beforeEach(() => {
  mocks.activeSessionId = "";
  mocks.activeWorkspace = { status: "ready", cwd: undefined };
  mocks.runtimeAvailable = true;
  mocks.open.mockReset();
  mocks.create.mockReset().mockResolvedValue("session-new");
  mocks.focusComposer.mockReset();
  mocks.notifyError.mockReset();
  disposePicker = configureWorkingDirectoryPicker({ open: mocks.open });
});

afterEach(() => disposePicker());

describe("useWorkIndexActions directory selection", () => {
  it("opens workspace selection when creating the first Session", () => {
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    expect(result.current.canCreateSession).toBe(true);
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.open).toHaveBeenCalledOnce();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("withdraws every Session mutation while Runtime commands are unavailable", () => {
    mocks.runtimeAvailable = false;
    const { result } = renderHook(() => useWorkIndexActions());

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
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith());
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });

  it("does not invent a default project while the active Session is resolving", () => {
    mocks.activeSessionId = "session-current";
    mocks.activeWorkspace = { status: "resolving", sessionId: "session-current" };
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    expect(result.current.canCreateSession).toBe(false);
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("opens the runtime workspace selector for a different directory", () => {
    const { result } = renderHook(() => useWorkIndexActions());
    act(() => result.current.chooseSessionFolder());
    expect(mocks.open).toHaveBeenCalledOnce();
    expect(mocks.create).not.toHaveBeenCalled();
  });

  it("keeps focus in the current session when project creation is rejected", async () => {
    mocks.create.mockResolvedValue(null);
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.startSessionInFolder("/tmp/project"));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ cwd: "/tmp/project" }));
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });
});
