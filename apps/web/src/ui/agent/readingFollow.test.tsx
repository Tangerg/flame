import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { createPortal } from "react-dom";
import { type ReactNode } from "react";
import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type StickToBottomInstance } from "use-stick-to-bottom";
import { Disclosure } from "@flame/ui";
import { prepareDisclosureReading } from "./readingFollow";

function followModel() {
  const scrollRef: StickToBottomInstance["scrollRef"] = Object.assign(
    (element: HTMLElement | null) => {
      scrollRef.current = element;
    },
    { current: null as HTMLElement | null },
  );
  const state: StickToBottomInstance["state"] = {
    scrollTop: 0,
    targetScrollTop: 0,
    calculatedTargetScrollTop: 0,
    scrollDifference: 0,
    resizeDifference: 0,
    velocity: 0,
    accumulated: 0,
    escapedFromLock: false,
    isAtBottom: true,
    isNearBottom: true,
  };
  return { scrollRef, state, stopScroll: vi.fn() };
}

function mountReading(children: ReactNode) {
  const follow = followModel();
  const { container } = testClient.render(
    <div ref={follow.scrollRef}>
      {/* Native button activations bubble here; this observer is not another control. */}
      {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div onClick={(event) => prepareDisclosureReading(event, follow)}>{children}</div>
    </div>,
  );
  return { follow, container };
}

function closingRecord(top: number, height = 32) {
  const { follow } = mountReading(
    <>
      <input aria-label="Current draft" />
      <Disclosure defaultOpen title="Recorded operation">
        Original receipt
      </Disclosure>
    </>,
  );
  const viewport = follow.scrollRef.current!;
  viewport.style.scrollPaddingTop = "24px";
  viewport.style.scrollPaddingBottom = "80px";
  vi.spyOn(viewport, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 40, 200, 300));
  const trigger = screen.getByRole("button", { name: "Recorded operation" });
  vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue(new DOMRect(0, top, 180, height));
  const reveal = vi.spyOn(trigger, "scrollIntoView").mockImplementation(() => undefined);
  return { follow, trigger, reveal };
}

describe("transcript disclosure engagement", () => {
  afterEach(() => vi.restoreAllMocks());

  it("retires follow before a real closed Base UI disclosure mounts its evidence", () => {
    const { follow } = mountReading(
      <Disclosure title="Recorded operation">Original receipt</Disclosure>,
    );
    const trigger = screen.getByRole("button", { name: "Recorded operation" });
    expect(trigger.hasAttribute("aria-controls")).toBe(false);
    trigger.focus();
    fireEvent.click(trigger.querySelector("span")!);
    expect(follow.stopScroll).toHaveBeenCalledTimes(1);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Original receipt")).toBeTruthy();
    expect(document.activeElement).toBe(trigger);

    fireEvent.click(trigger);
    expect(follow.stopScroll).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["ordinary action", {}],
    ["closing disclosure", { "aria-expanded": true }],
    ["native disabled", { "aria-expanded": false, disabled: true }],
    ["focusable disabled", { "aria-expanded": false, "aria-disabled": true }],
    ["menu", { "aria-expanded": false, "aria-haspopup": "menu" as const }],
    ["dialog", { "aria-expanded": false, "aria-haspopup": "dialog" as const }],
    ["combobox", { "aria-expanded": false, role: "combobox" }],
  ])("preserves follow for %s", (_, props) => {
    const { follow } = mountReading(<button {...props}>Another action</button>);
    fireEvent.click(screen.getByText("Another action"));
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("accepts explicit no-popup semantics without changing another control's focus", () => {
    const { follow } = mountReading(
      <>
        <input aria-label="Current draft" />
        <button aria-expanded={false} aria-haspopup="false">
          Read record
        </button>
      </>,
    );
    const draft = screen.getByRole("textbox");
    draft.focus();
    fireEvent.click(screen.getByRole("button", { name: "Read record" }));
    expect(follow.stopScroll).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(draft);
  });

  it("honors a prevented activation", () => {
    const { follow } = mountReading(
      <button aria-expanded={false} onClick={(event) => event.preventDefault()}>
        Read record
      </button>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Read record" }));
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("preserves a reading position that already escaped follow", () => {
    const { follow } = mountReading(<button aria-expanded={false}>Read record</button>);
    follow.state.isAtBottom = false;
    fireEvent.click(screen.getByRole("button", { name: "Read record" }));
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("ignores an activation while the retained viewport is detached", () => {
    const { follow } = mountReading(<button aria-expanded={false}>Read record</button>);
    follow.scrollRef.current = null;
    fireEvent.click(screen.getByRole("button", { name: "Read record" }));
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("ignores a portaled control outside this reader", () => {
    const { follow } = mountReading(
      createPortal(<button aria-expanded={false}>Outside record</button>, document.body),
    );
    fireEvent.click(screen.getByRole("button", { name: "Outside record" }));
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it.each([
    ["above the reader", 10, 32, "nearest"],
    ["below the reader", 350, 32, "nearest"],
    ["inside the header clearance", 52, 32, "nearest"],
    ["inside the Composer clearance", 245, 32, "nearest"],
    ["taller than the safe reading area", 40, 260, "start"],
  ] as const)("reveals a focused closing record %s", (_, top, height, block) => {
    const { follow, trigger, reveal } = closingRecord(top, height);
    follow.state.isAtBottom = false;
    act(() => trigger.focus());

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Original receipt")).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(reveal).toHaveBeenCalledOnce();
    expect(reveal).toHaveBeenCalledWith({ block, inline: "nearest", behavior: "instant" });
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("retires pending follow before revealing the focused closing record", () => {
    const { follow, trigger, reveal } = closingRecord(10);
    const order: string[] = [];
    follow.stopScroll.mockImplementation(() => order.push("follow retired"));
    reveal.mockImplementation(() => order.push("record revealed"));
    act(() => trigger.focus());

    fireEvent.click(trigger);

    expect(order).toEqual(["follow retired", "record revealed"]);
    expect(document.activeElement).toBe(trigger);
  });

  it("keeps an already visible closing record in place", () => {
    const { follow, trigger, reveal } = closingRecord(100);
    act(() => trigger.focus());

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(trigger);
    expect(reveal).not.toHaveBeenCalled();
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it("preserves a later draft focus while another record closes", () => {
    const { follow, trigger, reveal } = closingRecord(10);
    const draft = screen.getByRole("textbox", { name: "Current draft" });
    act(() => draft.focus());

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(draft);
    expect(reveal).not.toHaveBeenCalled();
    expect(follow.stopScroll).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    "leaves a footer toggle's reading position with its owner (linked: %s)",
    (linked) => {
      const { follow } = mountReading(
        <>
          <div id="earlier-content">Original long message</div>
          <button aria-expanded={true} aria-controls={linked ? "earlier-content" : undefined}>
            Show less
          </button>
        </>,
      );
      vi.spyOn(follow.scrollRef.current!, "getBoundingClientRect").mockReturnValue(
        new DOMRect(0, 40, 200, 300),
      );
      const trigger = screen.getByRole("button", { name: "Show less" });
      vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 180, 32));
      const reveal = vi.spyOn(trigger, "scrollIntoView").mockImplementation(() => undefined);
      act(() => trigger.focus());

      fireEvent.click(trigger);

      expect(document.activeElement).toBe(trigger);
      expect(reveal).not.toHaveBeenCalled();
      expect(follow.stopScroll).not.toHaveBeenCalled();
    },
  );
});
