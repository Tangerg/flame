import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentActivityDisclosure } from "@/ui/agent/activity-disclosure";
describe("AgentActivityDisclosure", () => {
  it("owns one accessible summary and detail region", () => {
    const onOpenChange = vi.fn();
    const { rerender } = testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="Search source"
        detail="runtime"
        open={false}
        onOpenChange={onOpenChange}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: /Search source/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    const shut = screen.getByRole("region", { hidden: true });
    expect(trigger.getAttribute("aria-controls")).toBeNull();
    expect(screen.queryByRole("region", { name: /Search source/ })).toBeNull();
    expect(shut.textContent).toBe("");

    fireEvent.click(trigger);
    expect(onOpenChange).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(true);

    rerender(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="Search source"
        detail="runtime"
        open
        onOpenChange={onOpenChange}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const region = screen.getByRole("region");
    expect(region.getAttribute("aria-labelledby")).toBe(trigger.id);
    expect(screen.getByText("Search result")).toBeTruthy();
  });

  it("names the toggle by its summary and detail, never by a changing preview", () => {
    const { rerender } = testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Search source"
        detail="runtime"
        open={false}
        onOpenChange={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    expect(screen.getByRole("button", { name: /Search source.*runtime/ })).toBeTruthy();

    for (const glimpse of ["first line", "second line"]) {
      rerender(
        <AgentActivityDisclosure
          icon="brain"
          shell="line"
          label="Thinking"
          preview={glimpse}
          open={false}
          onOpenChange={() => {}}
        >
          body
        </AgentActivityDisclosure>,
      );
      const toggle = screen.getByRole("button", { name: "Thinking" });
      expect(screen.getByText(glimpse), "the preview stays on screen").toBeTruthy();
      expect(toggle.textContent).not.toContain(glimpse);
    }
  });

  it("lets a caller describe its actions by the summary it already shows", () => {
    const actions = (
      <button type="button" aria-describedby="caller-summary">
        Copy
      </button>
    );
    const { rerender } = testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Search source"
        summaryId="caller-summary"
        actions={actions}
        open
        onOpenChange={() => {}}
      >
        <p>Search result</p>
      </AgentActivityDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: "Search source" });
    expect(screen.getByRole("region", { name: "Search source" }).id).toBe(
      trigger.getAttribute("aria-controls"),
    );
    screen.getByRole("button", { name: "Copy", description: "Search source" });

    rerender(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Denied search"
        summaryId="caller-summary"
        actions={actions}
        open={false}
        onOpenChange={() => {}}
      />,
    );

    expect(screen.queryByRole("button", { name: /Denied search/ })).toBeNull();
    screen.getByRole("button", { name: "Copy", description: "Denied search" });
  });

  it("keeps row actions outside the disclosure trigger", () => {
    const onOpenChange = vi.fn();
    const onAction = vi.fn();
    testClient.render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        shell="card"
        label="Delegated run"
        open={false}
        onOpenChange={onOpenChange}
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
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("gives each shell its own material", () => {
    const shells = (["line", "card"] as const).map((shell) => {
      const { unmount } = testClient.render(
        <AgentActivityDisclosure
          icon="search"
          shell={shell}
          label={shell}
          open={false}
          onOpenChange={() => {}}
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
    const { unmount } = testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="card"
        label="framed"
        open={false}
        onOpenChange={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const framed = screen
      .getByRole("button", { name: "framed" })
      .querySelector("span[aria-hidden]");
    expect(framed?.getAttribute("data-framed")).toBe("");
    unmount();

    testClient.render(
      <AgentActivityDisclosure
        leading={<span>•</span>}
        shell="card"
        label="own"
        open={false}
        onOpenChange={() => {}}
      >
        body
      </AgentActivityDisclosure>,
    );
    const own = screen.getByRole("button", { name: /own/ }).querySelector("span[aria-hidden]");
    expect(own?.getAttribute("data-framed")).toBeNull();
  });

  it("keeps a line shell's identity glyph visible without card chrome", () => {
    testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="quiet search"
        open={false}
        onOpenChange={() => {}}
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
    const { rerender } = testClient.render(
      <AgentActivityDisclosure
        icon="search"
        shell="line"
        label="Searched files"
        trailing="3 steps"
        open={false}
        onOpenChange={() => {}}
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
        onOpenChange={() => {}}
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
