import { describe, expect, it, vi } from "vitest";
import { createLocalWorkspaceActions } from "@/features/workspace/adapters/localWorkspaceActions";
describe("local workspace actions", () => {
  it("rechecks locality when a native action is invoked after a Runtime change", async () => {
    const openPath = vi.fn().mockResolvedValue(true);
    const revealPath = vi.fn().mockResolvedValue(true);
    let local = true;
    const actions = createLocalWorkspaceActions({ openPath, revealPath }, () => local);
    expect(actions.available()).toBe(true);
    await expect(actions.open("/repo/", "src/main.go")).resolves.toBe(true);
    expect(openPath).toHaveBeenCalledExactlyOnceWith("/repo/src/main.go");
    local = false;
    expect(actions.available()).toBe(false);
    await expect(actions.open("/repo", "src/main.go")).resolves.toBe(false);
    await expect(actions.reveal("/repo", "src/main.go")).resolves.toBe(false);
    expect(openPath).toHaveBeenCalledOnce();
    expect(revealPath).not.toHaveBeenCalled();
  });
  it("preserves an absolute path supplied by the local Runtime", async () => {
    const revealPath = vi.fn().mockResolvedValue(true);
    const actions = createLocalWorkspaceActions({ openPath: vi.fn(), revealPath }, () => true);
    await expect(actions.reveal("/repo", "/repo/assets/logo.png")).resolves.toBe(true);
    expect(revealPath).toHaveBeenCalledExactlyOnceWith("/repo/assets/logo.png");
  });
  it("keeps an action bound to the host that created it across renderer replacement", async () => {
    const retiredOpen = vi.fn();
    const retired = createLocalWorkspaceActions(
      { openPath: retiredOpen, revealPath: vi.fn() },
      () => false,
    );
    const openPath = vi.fn().mockResolvedValue(true);
    const successor = createLocalWorkspaceActions({ openPath, revealPath: vi.fn() }, () => true);
    await expect(retired.open("/repo", "file.txt")).resolves.toBe(false);
    await expect(successor.open("/repo", "file.txt")).resolves.toBe(true);
    expect(retiredOpen).not.toHaveBeenCalled();
    expect(openPath).toHaveBeenCalledExactlyOnceWith("/repo/file.txt");
  });
});
