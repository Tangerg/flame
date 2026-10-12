import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Brain } from "@flame/icons/react";
import {
  Button,
  Disclosure,
  Checkbox,
  FontScaleProvider,
  IconButton,
  SearchField,
  TextArea,
  TooltipProvider,
} from "../dist/index.js";
import { vocab } from "../dist/vocabulary.js";

afterEach(cleanup);
describe("shared controls", () => {
  it("keeps pending buttons keyboard focusable and blocks submission", () => {
    const click = vi.fn();
    render(
      <Button pending onClick={click}>
        Save
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Save" });
    fireEvent.click(button);
    expect(click).not.toHaveBeenCalled();
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(button.getAttribute("disabled")).toBeNull();
  });
  it("uses one StyleX merge for control overrides", () => {
    render(<Button styles={vocab.negative}>Delete</Button>);
    expect(screen.getByRole("button").className).toContain(
      Object.values(vocab.negative).find((value) => typeof value === "string"),
    );
  });
  it("keeps disclosure content lazy and exposes its expanded state", async () => {
    const displayed = vi.fn();
    function Evidence() {
      displayed();
      return <p>Recorded evidence</p>;
    }
    render(
      <Disclosure title="Inspect observation">
        <Evidence />
      </Disclosure>,
    );
    const trigger = screen.getByRole("button", { name: "Inspect observation" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(displayed).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    await screen.findByText("Recorded evidence");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
  });
  it("keeps checkbox changes at the controlled consumer", () => {
    const change = vi.fn();
    render(
      <Checkbox checked={false} onCheckedChange={change} label="Pinned only" />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Pinned only" }));
    expect(change).toHaveBeenCalledWith(true, expect.anything());
  });
  it("uses the consumer's font scale with the icon library's optical master", () => {
    const view = render(
      <FontScaleProvider fontSize={18}>
        <TooltipProvider>
          <IconButton icon={Brain} title="Memory" />
        </TooltipProvider>
      </FontScaleProvider>,
    );
    expect(
      screen
        .getByRole("button", { name: "Memory" })
        .querySelector("svg")
        ?.getAttribute("width"),
    ).toBe("20");
    view.rerender(
      <FontScaleProvider fontSize={11}>
        <TooltipProvider>
          <IconButton icon={Brain} title="Memory" />
        </TooltipProvider>
      </FontScaleProvider>,
    );
    expect(
      screen
        .getByRole("button", { name: "Memory" })
        .querySelector("svg")
        ?.getAttribute("width"),
    ).toBe("13");
  });
  it("preserves readonly fields while a write is pending", () => {
    render(
      <TextArea
        pending
        value="Recorded"
        aria-label="Fact"
        onChange={() => {}}
      />,
    );
    expect(
      (screen.getByRole("textbox", { name: "Fact" }) as HTMLTextAreaElement)
        .readOnly,
    ).toBe(true);
  });
  it("clears search through an explicitly named control", () => {
    const clear = vi.fn();
    render(
      <SearchField
        value="fact"
        aria-label="Search"
        onChange={() => {}}
        onClear={clear}
        clearLabel="Clear search"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(clear).toHaveBeenCalledOnce();
  });
});
