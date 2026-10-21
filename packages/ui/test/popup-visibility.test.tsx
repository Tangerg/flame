import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ActionMenu, ContentVisibility, Select } from "../dist/index.js";

afterEach(cleanup);

function Host({
  visible,
  children,
}: {
  visible: boolean;
  children: ReactNode;
}) {
  return (
    <>
      <button>Outside</button>
      <div hidden={!visible}>
        <ContentVisibility visible={visible}>
          <ContentVisibility visible>{children}</ContentVisibility>
        </ContentVisibility>
      </div>
    </>
  );
}

function mount(control: ReactNode) {
  const view = render(<Host visible>{control}</Host>);
  return {
    hide: () => view.rerender(<Host visible={false}>{control}</Host>),
    reveal: () => view.rerender(<Host visible>{control}</Host>),
  };
}

const options = [
  { value: "split", label: "Split" },
  { value: "unified", label: "Unified" },
];

async function openPopup(trigger: HTMLElement, role: "listbox" | "menu") {
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const popup = await screen.findByRole(role);
  await vi.waitFor(() =>
    expect(popup.contains(document.activeElement)).toBe(true),
  );
  return popup;
}

describe("Select retirement", () => {
  const select = (onValueChange: (value: string) => void) => (
    <Select
      label="Diff layout"
      value="split"
      onValueChange={onValueChange}
      options={options}
    />
  );

  it("closes without choosing when its host hides and stays closed on reveal", async () => {
    const change = vi.fn();
    const host = mount(select(change));
    const trigger = screen.getByRole("combobox", { name: "Diff layout" });
    await openPopup(trigger, "listbox");

    host.hide();

    expect(document.activeElement).toBe(document.body);
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(document.activeElement).not.toBe(trigger);

    host.reveal();

    expect(screen.getByRole("combobox", { name: "Diff layout" })).toBe(trigger);
    expect(trigger.textContent).toContain("Split");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(change).not.toHaveBeenCalled();
  });

  it("keeps focus that moved outside while the host hides", async () => {
    const host = mount(select(() => {}));
    await openPopup(screen.getByRole("combobox"), "listbox");
    const outside = screen.getByRole("button", { name: "Outside" });

    act(() => {
      host.hide();
      outside.focus();
    });
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());

    expect(document.activeElement).toBe(outside);
  });

  it("still chooses and dismisses with Escape while visible", async () => {
    const change = vi.fn();
    mount(select(change));
    const trigger = screen.getByRole("combobox");

    const listbox = await openPopup(trigger, "listbox");
    const option = within(listbox).getByRole("option", { name: "Unified" });
    fireEvent.pointerDown(option, { pointerType: "mouse" });
    fireEvent.click(option);
    expect(change).toHaveBeenCalledExactlyOnceWith("unified");
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());

    await openPopup(trigger, "listbox");
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(change).toHaveBeenCalledOnce();
  });
});

describe("ActionMenu retirement", () => {
  const menu = (onSelect: (opener: HTMLButtonElement) => void) => (
    <ActionMenu
      label="Memory actions"
      items={[{ label: "Forget", onSelect }]}
    />
  );

  it("closes without acting when its host hides and stays closed on reveal", async () => {
    const forget = vi.fn();
    const host = mount(menu(forget));
    const trigger = screen.getByRole("button", { name: "Memory actions" });
    const popup = await openPopup(trigger, "menu");

    host.hide();

    expect(document.activeElement).toBe(document.body);
    await vi.waitFor(() => expect(popup.isConnected).toBe(false));
    expect(document.activeElement).not.toBe(trigger);

    host.reveal();

    expect(screen.getByRole("button", { name: "Memory actions" })).toBe(
      trigger,
    );
    expect(screen.queryByRole("menu")).toBeNull();
    expect(forget).not.toHaveBeenCalled();
  });

  it("keeps focus that moved outside while the host hides", async () => {
    const host = mount(menu(() => {}));
    const popup = await openPopup(
      screen.getByRole("button", { name: "Memory actions" }),
      "menu",
    );
    const outside = screen.getByRole("button", { name: "Outside" });

    act(() => {
      host.hide();
      outside.focus();
    });
    await vi.waitFor(() => expect(popup.isConnected).toBe(false));

    expect(document.activeElement).toBe(outside);
  });

  it("still acts with its opener and dismisses with Escape while visible", async () => {
    const forget = vi.fn();
    mount(menu(forget));
    const trigger = screen.getByRole("button", { name: "Memory actions" });

    let popup = await openPopup(trigger, "menu");
    fireEvent.click(within(popup).getByRole("menuitem", { name: "Forget" }));
    expect(forget).toHaveBeenCalledExactlyOnceWith(trigger);
    await vi.waitFor(() => expect(popup.isConnected).toBe(false));

    popup = await openPopup(trigger, "menu");
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    await vi.waitFor(() => expect(popup.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(forget).toHaveBeenCalledOnce();
  });
});
