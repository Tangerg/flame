import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { useState, type ComponentProps } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Select } from "../dist/index.js";

beforeEach(() => {
  // Happy DOM has no layout; an open anchor needs a real nonzero box for Base UI.
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      return this === document.documentElement || this === document.body
        ? new DOMRect(0, 0, 1024, 768)
        : new DOMRect(20, 20, 160, 32);
    },
  );
  for (const [property, field, viewport] of [
    ["clientWidth", 160, 1024],
    ["clientHeight", 32, 768],
  ] as const) {
    vi.spyOn(HTMLElement.prototype, property, "get").mockImplementation(
      function (this: HTMLElement) {
        return this === document.documentElement || this === document.body
          ? viewport
          : field;
      },
    );
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const options = [
  { value: "split", label: "Split" },
  { value: "unified", label: "Unified" },
];

type SelectProps = ComponentProps<typeof Select>;

function mount(props: Partial<SelectProps>) {
  const change = vi.fn();
  const submit = vi.fn((event: { preventDefault(): void }) =>
    event.preventDefault(),
  );
  const keys = vi.fn((event: { key: string; defaultPrevented: boolean }) => [
    event.key,
    event.defaultPrevented,
  ]);
  const field = (next: Partial<SelectProps>) => (
    <form onSubmit={submit} onKeyDown={keys}>
      <span id="layout-note">Applies to every diff</span>
      <Select
        label="Diff layout"
        aria-describedby="layout-note"
        value="split"
        options={options}
        onValueChange={change}
        {...next}
      />
    </form>
  );
  const view = render(field(props));
  return {
    change,
    submit,
    keys,
    trigger: screen.getByRole("combobox", { name: "Diff layout" }),
    rerender: (next: Partial<SelectProps>) => view.rerender(field(next)),
  };
}

// The consumer owns the accepted value; the Select is only locked while its commit is unsettled.
function PendingChoice({
  commit,
}: {
  commit: (value: string) => Promise<string>;
}) {
  const [value, setValue] = useState("split");
  const [pending, setPending] = useState(false);
  return (
    <>
      <Select
        label="Diff layout"
        value={value}
        options={options}
        disabled={pending}
        focusableWhenDisabled
        onValueChange={(next) => {
          setPending(true);
          void commit(next).then((accepted) => {
            setValue(accepted);
            setPending(false);
          });
        }}
      />
      <button type="button">Outside</button>
    </>
  );
}

function mountPending() {
  const settled = Promise.withResolvers<string>();
  const commit = vi.fn((_value: string) => settled.promise);
  render(<PendingChoice commit={commit} />);
  return {
    commit,
    settled,
    trigger: screen.getByRole("combobox", { name: "Diff layout" }),
  };
}

async function choose(trigger: HTMLElement, label: string) {
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const listbox = await screen.findByRole("listbox");
  await vi.waitFor(() =>
    expect(listbox.contains(document.activeElement)).toBe(true),
  );
  const option = within(listbox).getByRole("option", { name: label });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
}

function attemptChoice(trigger: HTMLElement) {
  fireEvent.pointerDown(trigger, { pointerType: "mouse" });
  fireEvent.mouseDown(trigger);
  fireEvent.click(trigger);
  for (const key of ["Enter", " ", "ArrowDown", "ArrowUp", "Home", "End", "u"])
    fireEvent.keyDown(trigger, { key });
  fireEvent.keyUp(trigger, { key: " " });
}

function expectFocusablyLocked(trigger: HTMLElement) {
  expect(trigger.getAttribute("aria-disabled")).toBe("true");
  expect(trigger.hasAttribute("disabled")).toBe(false);
  expect(trigger.tabIndex).toBe(0);
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
}

it("keeps a focused trigger's identity, value and description when it becomes focusably disabled", () => {
  const field = mount({ focusableWhenDisabled: true });
  act(() => field.trigger.focus());

  field.rerender({ focusableWhenDisabled: true, disabled: true });

  expect(screen.getByRole("combobox", { name: "Diff layout" })).toBe(
    field.trigger,
  );
  expect(document.activeElement).toBe(field.trigger);
  expect(field.trigger.textContent).toContain("Split");
  expect(field.trigger.getAttribute("aria-describedby")).toBe("layout-note");
  expectFocusablyLocked(field.trigger);
});

it("blocks opening, navigation, typeahead and submission while focusably disabled", async () => {
  const field = mount({ focusableWhenDisabled: true, disabled: true });
  act(() => field.trigger.focus());

  attemptChoice(field.trigger);
  await act(async () => {});

  expectFocusablyLocked(field.trigger);
  expect(document.activeElement).toBe(field.trigger);
  expect(field.trigger.textContent).toContain("Split");
  expect(field.change).not.toHaveBeenCalled();
  expect(field.submit).not.toHaveBeenCalled();
});

it("leaves unrelated keys to the enclosing owner while focusably disabled", () => {
  const field = mount({ focusableWhenDisabled: true, disabled: true });
  act(() => field.trigger.focus());

  expect(fireEvent.keyDown(field.trigger, { key: "Escape" })).toBe(true);
  expect(fireEvent.keyDown(field.trigger, { key: "Tab" })).toBe(true);

  expect(field.keys.mock.results.map((result) => result.value)).toEqual([
    ["Escape", false],
    ["Tab", false],
  ]);
  expect(document.activeElement).toBe(field.trigger);
});

it("returns focus to the trigger that a chosen option disables and selects normally after settlement", async () => {
  const { commit, settled, trigger } = mountPending();

  await choose(trigger, "Unified");

  expect(commit).toHaveBeenCalledExactlyOnceWith("unified");
  await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
  expectFocusablyLocked(trigger);
  expect(trigger.textContent).toContain("Split");
  attemptChoice(trigger);
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  expect(commit).toHaveBeenCalledOnce();

  await act(async () => settled.resolve("unified"));

  expect(trigger.getAttribute("aria-disabled")).not.toBe("true");
  expect(trigger.textContent).toContain("Unified");
  expect(document.activeElement).toBe(trigger);
  const next = Promise.withResolvers<string>();
  commit.mockReturnValueOnce(next.promise);
  await choose(trigger, "Split");
  expect(commit).toHaveBeenLastCalledWith("split");
  expect(commit).toHaveBeenCalledTimes(2);
});

it("does not reclaim later outside focus when the disabling command settles", async () => {
  const { settled, trigger } = mountPending();
  await choose(trigger, "Unified");
  await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
  const outside = screen.getByRole("button", { name: "Outside" });

  act(() => outside.focus());
  await act(async () => settled.resolve("unified"));

  expect(trigger.textContent).toContain("Unified");
  expect(trigger.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(outside);
});

it("keeps an ordinary disabled trigger natively disabled and out of the tab order", async () => {
  const field = mount({ disabled: true });

  attemptChoice(field.trigger);
  await act(async () => {});

  expect(field.trigger.hasAttribute("disabled")).toBe(true);
  expect(field.trigger.getAttribute("aria-disabled")).toBeNull();
  expect(field.trigger.tabIndex).toBe(-1);
  expect(field.trigger.getAttribute("aria-expanded")).toBe("false");
  expect(field.change).not.toHaveBeenCalled();
});
