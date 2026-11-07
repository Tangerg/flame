import type { ComponentProps } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Select } from "../dist/index.js";

const anchor = vi.hoisted(() => ({ hidden: false, pending: false }));

vi.mock("@base-ui/react/select", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@base-ui/react/select")>();
  return {
    ...actual,
    Select: {
      ...actual.Select,
      Positioner(props: ComponentProps<typeof actual.Select.Positioner>) {
        return (
          <actual.Select.Positioner
            {...props}
            render={(elementProps, state) =>
              typeof props.render === "function" ? (
                props.render(
                  {
                    ...elementProps,
                    style: {
                      ...elementProps.style,
                      ...(anchor.pending ? { opacity: 0 } : {}),
                    },
                  },
                  {
                    ...state,
                    anchorHidden: anchor.hidden,
                  },
                )
              ) : (
                <div {...elementProps} />
              )
            }
          />
        );
      },
    },
  };
});

afterEach(() => {
  cleanup();
  anchor.hidden = false;
  anchor.pending = false;
});

function mount() {
  const change = vi.fn();
  const content = () => (
    <>
      <button>Outside</button>
      <section aria-label="Settings" tabIndex={0} style={{ overflowY: "auto" }}>
        <Select
          label="Interface size"
          value="14"
          onValueChange={change}
          options={[
            { value: "14", label: "14" },
            { value: "18", label: "18" },
          ]}
        />
      </section>
    </>
  );
  const view = render(content());
  const owner = screen.getByRole("region", { name: "Settings" });
  Object.defineProperties(owner, {
    clientHeight: { configurable: true, value: 100 },
    scrollHeight: { configurable: true, value: 500 },
  });
  owner.scrollTop = 200;
  return { ...view, owner, change, update: () => view.rerender(content()) };
}

async function open() {
  const trigger = screen.getByRole("combobox", { name: "Interface size" });
  fireEvent.click(trigger);
  const popup = await screen.findByRole("listbox");
  await waitFor(() =>
    expect(popup.contains(document.activeElement)).toBe(true),
  );
  return trigger;
}

describe("detached Select anchor", () => {
  it("retires without choosing or scrolling back, then stays closed when the anchor returns", async () => {
    const view = mount();
    const trigger = await open();
    anchor.hidden = true;
    view.update();
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(view.owner));
    expect(view.owner.scrollTop).toBe(200);
    expect(view.change).not.toHaveBeenCalled();
    anchor.hidden = false;
    view.update();
    expect(screen.getByRole("combobox")).toBe(trigger);
    expect(trigger.textContent).toBe("14");
    expect(screen.queryByRole("listbox")).toBeNull();
    await open();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("preserves a later outside focus during retirement", async () => {
    const view = mount();
    await open();
    const outside = screen.getByRole("button", { name: "Outside" });
    act(() => {
      outside.focus();
      anchor.hidden = true;
      view.update();
    });
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(document.activeElement).toBe(outside);
    expect(view.owner.scrollTop).toBe(200);
    expect(view.change).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    "waits for the current position with a retained hidden sample (previously clipped: %s)",
    async (previouslyClipped) => {
      const view = mount();
      const trigger = screen.getByRole("combobox", { name: "Interface size" });
      if (previouslyClipped) {
        await open();
        anchor.hidden = true;
        view.update();
        await waitFor(() =>
          expect(trigger.getAttribute("aria-expanded")).toBe("false"),
        );
        await waitFor(() => expect(document.activeElement).toBe(view.owner));
      }
      anchor.hidden = true;
      anchor.pending = true;
      view.update();

      fireEvent.click(trigger);
      await waitFor(() =>
        expect(trigger.getAttribute("aria-expanded")).toBe("true"),
      );
      const popup = await screen.findByRole("listbox");
      const opacity = () => {
        let value = 1;
        for (let node: Element | null = popup; node; node = node.parentElement)
          value *= Number(getComputedStyle(node).opacity || 1);
        return value;
      };
      expect(opacity()).toBe(0);
      anchor.hidden = false;
      anchor.pending = false;
      view.update();

      await waitFor(() => expect(opacity()).toBe(1));
      await waitFor(() =>
        expect(popup.contains(document.activeElement)).toBe(true),
      );
      fireEvent.keyDown(document.activeElement!, { key: "Escape" });
      await waitFor(() => expect(document.activeElement).toBe(trigger));
      expect(trigger.textContent).toBe("14");
      expect(view.change).not.toHaveBeenCalled();
    },
  );
});
