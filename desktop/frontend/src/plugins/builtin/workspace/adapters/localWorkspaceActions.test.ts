import { afterEach, describe, expect, it, vi } from "vitest";
import { localWorkspace } from "../application/ports/localWorkspace";
import { installLocalWorkspaceActions } from "./localWorkspaceActions";

const disposers: Array<() => void> = [];

afterEach(() => {
  for (const dispose of disposers.splice(0).reverse()) dispose();
});

describe("local workspace actions", () => {
  it("rechecks locality when a native action is invoked after a Runtime change", async () => {
    const openPath = vi.fn().mockResolvedValue(true);
    const revealPath = vi.fn().mockResolvedValue(true);
    let local = true;
    disposers.push(installLocalWorkspaceActions({ openPath, revealPath }, () => local));

    expect(localWorkspace().available()).toBe(true);
    await expect(localWorkspace().open("/repo/", "src/main.go")).resolves.toBe(true);
    expect(openPath).toHaveBeenCalledExactlyOnceWith("/repo/src/main.go");

    local = false;
    expect(localWorkspace().available()).toBe(false);
    await expect(localWorkspace().open("/repo", "src/main.go")).resolves.toBe(false);
    await expect(localWorkspace().reveal("/repo", "src/main.go")).resolves.toBe(false);
    expect(openPath).toHaveBeenCalledOnce();
    expect(revealPath).not.toHaveBeenCalled();
  });

  it("preserves an absolute path supplied by the local Runtime", async () => {
    const revealPath = vi.fn().mockResolvedValue(true);
    disposers.push(installLocalWorkspaceActions({ openPath: vi.fn(), revealPath }, () => true));

    await expect(localWorkspace().reveal("/repo", "/repo/assets/logo.png")).resolves.toBe(true);
    expect(revealPath).toHaveBeenCalledExactlyOnceWith("/repo/assets/logo.png");
  });

  it("keeps the successor host published when a retired installation disposes", async () => {
    const retired = installLocalWorkspaceActions(
      { openPath: vi.fn(), revealPath: vi.fn() },
      () => false,
    );
    const openPath = vi.fn().mockResolvedValue(true);
    disposers.push(installLocalWorkspaceActions({ openPath, revealPath: vi.fn() }, () => true));
    retired();

    await expect(localWorkspace().open("/repo", "file.txt")).resolves.toBe(true);
    expect(openPath).toHaveBeenCalledExactlyOnceWith("/repo/file.txt");
  });
});
