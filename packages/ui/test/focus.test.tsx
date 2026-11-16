import { act, cleanup, render, screen } from "@testing-library/react";
import { useLayoutEffect, useRef, type RefObject } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ContentVisibility,
  FocusWitness,
  RemovalFocus,
  handoffFocus,
  useRemovalFocus,
} from "../dist/index.js";

afterEach(() => {
  cleanup();
  document.body.replaceChildren();
});

function controls() {
  const source = document.createElement("section");
  const button = document.createElement("button");
  const target = document.createElement("button");
  source.append(button);
  document.body.append(source, target);
  button.focus();
  return { source, button, target };
}

describe("removal focus witness", () => {
  it("settles once after the captured subtree is physically removed", () => {
    const { source, target } = controls();
    const witness = RemovalFocus.capture(source, () => true)!;
    const resolve = vi.fn(() => target);
    expect(witness.restore(resolve)).toBe(false);
    source.remove();
    expect(witness.restore(resolve)).toBe(false);
    expect(resolve).not.toHaveBeenCalled();
    const next = controls();
    const removed = RemovalFocus.capture(next.source, () => true)!;
    next.source.remove();
    expect(removed.restore(() => next.target)).toBe(true);
    expect(removed.restore(() => target)).toBe(false);
    expect(document.activeElement).toBe(next.target);
  });

  it("refuses later focus even when it returns to the body before restoration", () => {
    const { source, target } = controls();
    const witness = RemovalFocus.capture(source, () => true)!;
    source.remove();
    target.focus();
    target.blur();
    const resolve = vi.fn(() => target);
    expect(witness.restore(resolve)).toBe(false);
    expect(resolve).not.toHaveBeenCalled();
  });

  it.each([
    "hidden",
    "inert",
    "aria-hidden",
    "display",
    "visibility",
    "disconnected",
    "disabled",
  ])("refuses an unavailable %s destination", (reason) => {
    const { source, target } = controls();
    const witness = RemovalFocus.capture(source, () => true)!;
    source.remove();
    if (reason === "display") target.style.display = "none";
    else if (reason === "visibility") target.style.visibility = "hidden";
    else if (reason === "disconnected") target.remove();
    else target.setAttribute(reason, reason === "aria-hidden" ? "true" : "");
    expect(witness.restore(() => target)).toBe(false);
    expect(document.activeElement).toBe(document.body);
  });

  it("rejects a retired source without resolving a successor destination", () => {
    const { source, target } = controls();
    let current = true;
    const witness = RemovalFocus.capture(source, () => current)!;
    source.remove();
    current = false;
    const resolve = vi.fn(() => target);
    expect(witness.restore(resolve)).toBe(false);
    expect(resolve).not.toHaveBeenCalled();
  });

  it("captures only the original focused subtree", () => {
    const { source, target } = controls();
    target.focus();
    expect(RemovalFocus.capture(source, () => true)).toBeNull();
  });
});

function Removed({
  target,
  current = () => true,
}: {
  target: () => HTMLElement | null;
  current?: () => boolean;
}) {
  const source = useRef<HTMLButtonElement>(null);
  useRemovalFocus(source, target, current);
  return <button ref={source}>Remove task</button>;
}

it("waits for the opener's committed availability and ignores retained ref bindings", () => {
  function Scene({ open }: { open: boolean }) {
    const opener = useRef<HTMLButtonElement>(null);
    useLayoutEffect(() => {
      opener.current!.disabled = open;
    }, [open]);
    return (
      <>
        <button ref={opener}>Open task</button>
        {open && <Removed target={() => opener.current} />}
      </>
    );
  }
  const view = render(<Scene open />);
  act(() => screen.getByRole("button", { name: "Remove task" }).focus());
  view.rerender(<Scene open />);
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Remove task" }),
  );
  view.rerender(<Scene open={false} />);
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Open task" }),
  );
});

it("captures the current node when a retained owner replaces its DOM binding", () => {
  const target = document.createElement("button");
  document.body.append(target);
  const resolve = vi.fn(() => target);
  function Task({ replaced }: { replaced: boolean }) {
    const source = useRef<HTMLElement>(null);
    useRemovalFocus(source, resolve);
    return replaced ? (
      <section ref={source}>
        <button>Current task</button>
      </section>
    ) : (
      <article ref={source}>
        <button>Current task</button>
      </article>
    );
  }
  const view = render(<Task replaced={false} />);
  const original = screen.getByRole("button", { name: "Current task" });
  act(() => original.focus());
  view.rerender(<Task replaced />);
  expect(original.isConnected).toBe(false);
  expect(resolve).not.toHaveBeenCalled();
  act(() => screen.getByRole("button", { name: "Current task" }).focus());
  view.unmount();
  expect(resolve).toHaveBeenCalledExactlyOnceWith();
  expect(document.activeElement).toBe(target);
});

it("retains an attached ancestor boundary when its ref detaches before task cleanup", () => {
  const target = document.createElement("button");
  document.body.append(target);
  const resolve = vi.fn(() => target);
  function Task({ source }: { source: RefObject<HTMLElement | null> }) {
    useRemovalFocus(source, resolve);
    return <button>Current task</button>;
  }
  function Scene({ active }: { active: boolean }) {
    const source = useRef<HTMLElement>(null);
    return <section ref={source}>{active && <Task source={source} />}</section>;
  }
  const view = render(<Scene active={false} />);
  view.rerender(<Scene active />);
  act(() => screen.getByRole("button", { name: "Current task" }).focus());
  view.unmount();
  expect(resolve).toHaveBeenCalledExactlyOnceWith();
  expect(document.activeElement).toBe(target);
});

it("retires a retained hidden task before its later removal", () => {
  const target = document.createElement("button");
  document.body.append(target);
  const scene = (visible: boolean, open: boolean) => (
    <ContentVisibility visible={visible}>
      {open && <Removed target={() => target} />}
    </ContentVisibility>
  );
  const view = render(scene(true, true));
  act(() => screen.getByRole("button", { name: "Remove task" }).focus());
  view.rerender(scene(false, true));
  view.rerender(scene(false, false));
  expect(document.activeElement).toBe(document.body);
});

it("hands focused retry ownership to an available surviving host synchronously", () => {
  const { source, button, target } = controls();
  expect(handoffFocus(source, target)).toBe(true);
  expect(document.activeElement).toBe(target);
  button.focus();
  target.hidden = true;
  expect(handoffFocus(source, target)).toBe(false);
  expect(document.activeElement).toBe(button);
});

it("rejects a changed business source after layout commits and before passive restoration", () => {
  const target = document.createElement("button");
  document.body.append(target);
  const resolve = vi.fn(() => target);
  function Scene({ scope, open }: { scope: string; open: boolean }) {
    const source = useRef(scope);
    useLayoutEffect(() => {
      source.current = scope;
    });
    return (
      open && (
        <Removed target={resolve} current={() => source.current === scope} />
      )
    );
  }
  const view = render(<Scene scope="original" open />);
  act(() => screen.getByRole("button", { name: "Remove task" }).focus());
  view.rerender(<Scene scope="successor" open={false} />);
  expect(document.activeElement).toBe(document.body);
  expect(resolve).not.toHaveBeenCalled();
});

it("settles a held-focus delivery before a later native choice", () => {
  const { source, button, target } = controls();
  const witness = FocusWitness.capture(source, () => true)!;
  expect(witness.restore(() => target)).toBe(true);
  button.focus();
  expect(witness.restore(() => target)).toBe(false);
  expect(document.activeElement).toBe(button);
});

it("can claim original focus exactly once without looking up a successor target", () => {
  const { source } = controls();
  const witness = FocusWitness.capture(source, () => true)!;
  expect(witness.take()).toBe(true);
  expect(witness.take()).toBe(false);
});
