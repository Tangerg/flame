import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button } from "../dist/index.js";

afterEach(cleanup);

describe.each(["disabled", "pending"] as const)(
  "focusable %s buttons",
  (state) => {
    it("preserves the composed Base UI event contract for non-activation keys", () => {
      const keyDown = vi.fn((event) => event.preventBaseUIHandler());
      render(
        <Button
          {...{ [state]: true }}
          focusableWhenDisabled
          onKeyDown={keyDown}
        >
          Read details
        </Button>,
      );
      expect(
        fireEvent.keyDown(screen.getByRole("button"), { key: "Escape" }),
      ).toBe(true);
      expect(keyDown).toHaveBeenCalledOnce();
    });
    it.each([
      { key: "0", metaKey: true },
      { key: "+", metaKey: true },
      { key: "0", ctrlKey: true },
      { key: "f", metaKey: true },
      { key: "ArrowLeft" },
      { key: "ArrowRight" },
      { key: "Escape" },
      { key: "Tab" },
      { key: "Tab", shiftKey: true },
    ])("leaves $key and its modifiers with the command owner", (key) => {
      const received: boolean[] = [];
      const content = (disabled: boolean) => (
        <div onKeyDown={(event) => received.push(event.defaultPrevented)}>
          <Button {...{ [state]: disabled }} focusableWhenDisabled>
            Zoom out
          </Button>
        </div>
      );
      const view = render(content(false));
      const button = screen.getByRole<HTMLButtonElement>("button", {
        name: "Zoom out",
      });
      button.focus();
      view.rerender(content(true));

      expect(document.activeElement).toBe(button);
      expect(button.disabled).toBe(false);
      expect(button.getAttribute("aria-disabled")).toBe("true");
      expect(button.hasAttribute("data-disabled")).toBe(true);
      expect(fireEvent.keyDown(button, key)).toBe(true);
      expect(received).toEqual([false]);
    });

    it.each([true, false])(
      "blocks activation and submission with nativeButton=%s",
      (nativeButton) => {
        const click = vi.fn();
        const submit = vi.fn((event) => event.preventDefault());
        render(
          <form onSubmit={submit}>
            <Button
              {...{ [state]: true }}
              focusableWhenDisabled
              nativeButton={nativeButton}
              render={nativeButton ? undefined : <span />}
              type="submit"
              onClick={click}
            >
              Next
            </Button>
          </form>,
        );
        const button = screen.getByRole("button", { name: "Next" });
        for (const key of ["Enter", " "]) {
          expect(fireEvent.keyDown(button, { key })).toBe(false);
          fireEvent.keyUp(button, { key });
        }
        expect(fireEvent.click(button)).toBe(false);
        expect(click).not.toHaveBeenCalled();
        expect(submit).not.toHaveBeenCalled();
      },
    );

    it("preserves custom rendering, refs and the ordinary enabled behavior", () => {
      const ref = createRef<HTMLButtonElement>();
      const click = vi.fn();
      const keyDown = vi.fn();
      const states: boolean[] = [];
      const content = (disabled: boolean) => (
        <Button
          ref={ref}
          {...{ [state]: disabled }}
          focusableWhenDisabled
          onClick={click}
          onKeyDown={keyDown}
          render={(props, state) => {
            states.push(state.disabled);
            return <button {...props} data-custom="retained" />;
          }}
        >
          Zoom out
        </Button>
      );
      const view = render(content(true));
      const button = screen.getByRole<HTMLButtonElement>("button", {
        name: "Zoom out",
      });
      expect(ref.current).toBe(button);
      expect(button.getAttribute("data-custom")).toBe("retained");
      expect(states.at(-1)).toBe(true);
      expect(fireEvent.keyDown(button, { key: "0", metaKey: true })).toBe(true);
      expect(keyDown).toHaveBeenCalledOnce();

      view.rerender(content(false));
      expect(ref.current).toBe(button);
      expect(states.at(-1)).toBe(false);
      fireEvent.keyDown(button, { key: "0", metaKey: true });
      expect(keyDown).toHaveBeenCalledTimes(2);
      fireEvent.click(button);
      expect(click).toHaveBeenCalledOnce();
    });

    it("retains cancellation by an earlier command owner", () => {
      const received: boolean[] = [];
      render(
        <div
          onKeyDownCapture={(event) => event.preventDefault()}
          onKeyDown={(event) => received.push(event.defaultPrevented)}
        >
          <Button {...{ [state]: true }} focusableWhenDisabled>
            Zoom out
          </Button>
        </div>,
      );
      expect(
        fireEvent.keyDown(screen.getByRole("button"), {
          key: "0",
          metaKey: true,
        }),
      ).toBe(false);
      expect(received).toEqual([true]);
    });
  },
);
