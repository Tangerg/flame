import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentActivityDisclosure } from "./activity-disclosure";

describe("AgentActivityDisclosure", () => {
  it("owns one accessible summary and detail region", () => {
    const onToggle = vi.fn();
    const { rerender } = render(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="Search source"
        detail="runtime"
        open={false}
        onToggle={onToggle}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: /Search source/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region")).toBeNull();

    fireEvent.click(trigger);
    expect(onToggle).toHaveBeenCalledOnce();

    rerender(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="Search source"
        detail="runtime"
        open
        onToggle={onToggle}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const region = screen.getByRole("region");
    expect(region.getAttribute("aria-labelledby")).toBe(trigger.id);
    expect(screen.getByText("Search result")).toBeTruthy();
  });

  it("keeps row actions outside the disclosure trigger", () => {
    const onToggle = vi.fn();
    const onAction = vi.fn();
    render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        shell="card"
        label="Delegated run"
        open={false}
        onToggle={onToggle}
        actions={
          <button type="button" onClick={onAction}>
            Cancel
          </button>
        }
      >
        Child narrative
      </AgentActivityDisclosure>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onAction).toHaveBeenCalledOnce();
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("gives each shell its own material", () => {
    const shells = (["line", "card"] as const).map((shell) => {
      const { unmount } = render(
        <AgentActivityDisclosure
          icon="search"
          shell={shell}
          label={shell}
          open={false}
          onToggle={() => {}}
        >
          body
        </AgentActivityDisclosure>,
      );
      const row = screen.getByRole("button", { name: shell }).closest("[data-shell]");
      const result = { shell: row?.getAttribute("data-shell") };
      unmount();
      return result;
    });

    expect(shells).toEqual([{ shell: "line" }, { shell: "card" }]);
  });

  it("frames its own glyph on a card, and never a caller's mark", () => {
    const { unmount } = render(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="framed"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const framed = screen
      .getByRole("button", { name: "framed" })
      .querySelector("span[aria-hidden]");
    expect(framed?.getAttribute("data-framed")).toBe("");
    unmount();

    render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        shell="card"
        label="own"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const own = screen.getByRole("button", { name: /own/ }).querySelector("span[aria-hidden]");
    expect(own?.getAttribute("data-framed")).toBeNull();
  });

  it("keeps a line shell's identity glyph visible without card chrome", () => {
    render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="quiet search"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );

    const mark = screen
      .getByRole("button", { name: "quiet search" })
      .querySelector("span[aria-hidden]");
    expect(mark?.getAttribute("data-framed")).toBeNull();
    expect(mark?.getAttribute("data-tone")).toBe("neutral");
    expect(mark?.querySelector("svg")).toBeTruthy();
  });

  it("puts the identity first and a quiet disclosure after the summary", () => {
    const { rerender } = render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Searched files"
        trailing="3 steps"
        open={false}
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: /Searched files/ });
    const slots = Array.from(trigger.children)
      .map((child) => child.getAttribute("data-slot"))
      .filter(Boolean);
    expect(slots).toEqual([
      "agent-activity-mark",
      "agent-activity-label",
      "agent-activity-chevron",
    ]);
    const chevron = trigger.querySelector('[data-slot="agent-activity-chevron"]');
    expect(chevron?.getAttribute("data-open")).toBeNull();

    rerender(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Searched files"
        trailing="3 steps"
        open
        onToggle={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const openChevron = screen
      .getByRole("button", { name: /Searched files/ })
      .querySelector('[data-slot="agent-activity-chevron"]');
    expect(openChevron?.getAttribute("data-open")).toBe("");
  });
});
