import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentActivityDisclosure } from "@/ui/agent/activity-disclosure";
describe("AgentActivityDisclosure", () => {
  it("owns one accessible summary and detail region", () => {
    const onToggle = vi.fn();
    const { rerender } = testClient.render(
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

    const shut = document.getElementById(trigger.getAttribute("aria-controls") ?? "");
    expect(shut).not.toBeNull();
    expect(shut!.getAttribute("role")).toBe("region");
    expect(shut!.closest("[inert]"), "a shut region is not reachable").not.toBeNull();
    expect(shut!.textContent).toBe("");

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
        onToggle={() => {}}
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
        onToggle={() => {}}
      />,
    );

    expect(screen.queryByRole("button", { name: /Denied search/ })).toBeNull();
    screen.getByRole("button", { name: "Copy", description: "Denied search" });
  });

  it("keeps row actions outside the disclosure trigger", () => {
    const onToggle = vi.fn();
    const onAction = vi.fn();
    testClient.render(
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
      const { unmount } = testClient.render(
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
    const { unmount } = testClient.render(
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

    testClient.render(
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
    testClient.render(
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
    const { rerender } = testClient.render(
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

function readingScenario({ header = 30, viewport = 400, top = 24, bottom = 80 } = {}) {
  const observers: Array<{ nodes: Set<Element>; callback: ResizeObserverCallback }> = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      nodes = new Set<Element>();
      constructor(callback: ResizeObserverCallback) {
        observers.push({ nodes: this.nodes, callback });
      }
      observe(node: Element) {
        this.nodes.add(node);
      }
      unobserve(node: Element) {
        this.nodes.delete(node);
      }
      disconnect() {
        this.nodes.clear();
      }
    },
  );
  const shape = { header, viewport };
  function Harness({ sticky = true, label = "Tool group" }: { sticky?: boolean; label?: string }) {
    return (
      <>
        <input aria-label="Later draft" defaultValue="Keep this draft" />
        <div data-testid="viewport" style={{ overflowY: "auto" }}>
          <div data-testid="content">
            <AgentActivityDisclosure
              icon="search"
              shell="line"
              label={label}
              open
              onToggle={() => {}}
              stickyHeader={sticky}
            >
              <button type="button">Run command</button>
            </AgentActivityDisclosure>
          </div>
        </div>
      </>
    );
  }
  const view = testClient.render(<Harness />);
  const viewportNode = screen.getByTestId("viewport");
  const content = screen.getByTestId("content");
  const trigger = screen.getByRole("button", { name: "Tool group" });
  const headerNode = trigger.closest<HTMLElement>('[data-slot="agent-activity-header"]')!;
  const region = screen.getByRole("region", { name: "Tool group" });
  const body = region.parentElement!;
  region.style.lineHeight = "20px";
  region.style.paddingTop = "6px";
  viewportNode.style.scrollPaddingTop = `${top}px`;
  viewportNode.style.scrollPaddingBottom = `${bottom}px`;
  vi.spyOn(viewportNode, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, 46, 600, shape.viewport),
  );
  vi.spyOn(headerNode, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, 46, 600, shape.header),
  );
  const resize = (node: Element) =>
    act(() => {
      for (const observer of observers)
        if (observer.nodes.has(node))
          observer.callback(
            [
              {
                target: node,
                contentRect: node.getBoundingClientRect(),
                borderBoxSize: [],
                contentBoxSize: [],
                devicePixelContentBoxSize: [],
              },
            ],
            {} as ResizeObserver,
          );
    });
  resize(headerNode);
  const reading = () => ({
    sticky: headerNode.hasAttribute("data-sticky"),
    clearance: body.hasAttribute("data-activity-clearance"),
    reserved: body.style.getPropertyValue("--disclosure-header-height"),
  });
  const observed = (node: Element) => observers.some((observer) => observer.nodes.has(node));
  return {
    ...view,
    Harness,
    shape,
    viewport: viewportNode,
    content,
    header: headerNode,
    trigger,
    region,
    resize,
    reading,
    observed,
  };
}

describe("AgentActivityDisclosure reading space", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("reserves its sticky summary only while the safe reading frame keeps body room", () => {
    const s = readingScenario();
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "30px" });

    const command = screen.getByRole("button", { name: "Run command" });
    act(() => command.focus());
    s.rerender(<s.Harness label="Tool group" />);
    expect(s.reading(), "a render does not reset the measured decision").toEqual({
      sticky: true,
      clearance: true,
      reserved: "30px",
    });

    s.shape.viewport = 331.5;
    s.viewport.style.scrollPaddingBottom = "282px";
    s.resize(s.viewport);

    expect(s.reading()).toEqual({ sticky: false, clearance: false, reserved: "" });
    s.rerender(<s.Harness label="Tool group" />);
    expect(s.reading().sticky, "a render does not restore an ineffective summary").toBe(false);
    expect(document.activeElement).toBe(command);
    expect(s.trigger.getAttribute("aria-expanded")).toBe("true");
    expect(s.trigger.getAttribute("aria-controls")).toBe(s.region.id);
    expect(s.region.textContent).toBe("Run command");
  });

  it("follows Composer clearance and recovered room without a viewport resize", () => {
    const s = readingScenario();
    const command = screen.getByRole("button", { name: "Run command" });
    act(() => command.focus());
    const draft = screen.getByRole<HTMLInputElement>("textbox", { name: "Later draft" });
    act(() => draft.focus());

    s.viewport.style.scrollPaddingBottom = "340px";
    s.resize(s.content);
    expect(s.reading()).toEqual({ sticky: false, clearance: false, reserved: "" });

    s.viewport.style.scrollPaddingBottom = "80px";
    s.resize(s.content);
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "30px" });

    s.shape.header = 280;
    s.resize(s.header);
    expect(s.reading(), "an oversized summary scrolls with its body").toEqual({
      sticky: false,
      clearance: false,
      reserved: "",
    });

    s.shape.header = 59;
    s.resize(s.header);
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "59px" });
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft");
  });

  it("keeps two inset body lines when a taller summary still fits", () => {
    const s = readingScenario({ header: 59, viewport: 214 });
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "59px" });

    s.shape.viewport = 208;
    s.resize(s.viewport);
    expect(s.reading()).toEqual({ sticky: false, clearance: false, reserved: "" });

    s.shape.viewport = 214;
    s.resize(s.viewport);
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "59px" });

    s.region.style.lineHeight = "normal";
    s.resize(s.content);
    expect(s.reading()).toEqual({ sticky: false, clearance: false, reserved: "" });
  });

  it("keeps an ordinary disclosure in flow and releases its observers", () => {
    const s = readingScenario();
    expect(s.observed(s.viewport) && s.observed(s.content) && s.observed(s.header)).toBe(true);

    s.rerender(<s.Harness sticky={false} />);
    expect(s.reading()).toEqual({ sticky: false, clearance: false, reserved: "" });
    expect([s.observed(s.viewport), s.observed(s.content), s.observed(s.header)]).toEqual([
      false,
      false,
      false,
    ]);

    s.rerender(<s.Harness />);
    expect(s.reading()).toEqual({ sticky: true, clearance: true, reserved: "30px" });

    s.unmount();
    expect([s.observed(s.viewport), s.observed(s.content), s.observed(s.header)]).toEqual([
      false,
      false,
      false,
    ]);
  });
});
