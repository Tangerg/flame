import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkIndexActions, WorkSession } from "@/plugins/builtin/navigation/public/workIndex";
import { SessionList } from "./SessionList";

const session = (id: number): WorkSession => ({
  id: `s${id}`,
  revision: 1,
  title: `Session ${id}`,
  time: new Date(2026, 0, 30 - id).toISOString(),
  attention: "none",
  favorite: false,
});

const actions = {
  selectSession: vi.fn(),
  renameSession: vi.fn(),
  forkSession: vi.fn(),
  deleteSession: vi.fn(),
  toggleFavorite: vi.fn(),
} as unknown as WorkIndexActions;

describe("SessionList", () => {
  it("keeps the active session on screen even past the visible cap", () => {
    const sessions = Array.from({ length: 12 }, (_, index) => session(index + 1));
    render(<SessionList sessions={sessions} actions={actions} activeSessionId="s11" />);
    expect(screen.getByRole("button", { name: /^Session 11 —/ }).getAttribute("aria-current")).toBe(
      "page",
    );
    expect(screen.queryByRole("button", { name: /^Session 10 —/ })).toBeNull();
  });
});
