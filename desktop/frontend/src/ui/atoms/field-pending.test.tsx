import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ChoiceList, ChoiceOption } from "./choice-list";
import { TextArea, TextField } from "./text-field";

// The other half of the `disabled`-means-unfocusable defect. `Button`'s `pending` fixed the
// controls that START async work; these are the controls the work happens FROM, and they lost
// focus the same way — measured on the relocate banner: type a path, press Enter, and focus is
// on `<body>` 120ms later.
//
// A field needs more than `aria-disabled`, because that alone leaves it typable. `readOnly` is
// what actually stops the edit while keeping the element focusable and its caret in place.
describe("field pending", () => {
  it("keeps a text field focusable and unwritable while its action is in flight", () => {
    render(<TextField aria-label="Path" defaultValue="/repo" pending />);
    const field = screen.getByRole("textbox", { name: "Path" }) as HTMLInputElement;

    expect(field.disabled).toBe(false);
    expect(field.readOnly).toBe(true);
    expect(field.getAttribute("aria-disabled")).toBe("true");

    field.focus();
    expect(document.activeElement).toBe(field);
  });

  it("keeps a textarea focusable and unwritable while its action is in flight", () => {
    render(<TextArea aria-label="Note" defaultValue="hello" pending />);
    const area = screen.getByRole("textbox", { name: "Note" }) as HTMLTextAreaElement;

    expect(area.disabled).toBe(false);
    expect(area.readOnly).toBe(true);
    expect(area.getAttribute("aria-disabled")).toBe("true");
  });

  it("writes normally when it is not pending", () => {
    render(<TextField aria-label="Path" />);
    const field = screen.getByRole("textbox", { name: "Path" }) as HTMLInputElement;
    expect(field.readOnly).toBe(false);
    expect(field.getAttribute("aria-disabled")).toBeNull();
  });

  // An answer is submitted BY choosing, so the list that submitted it is the list the keyboard
  // was on. `disabled` would take the whole group out of the tab order mid-answer.
  it("keeps a choice list focusable and refuses a second answer while pending", () => {
    const onValueChange = vi.fn();
    render(
      <ChoiceList
        multiple={false}
        value={["a"]}
        values={["a", "b"]}
        labelledBy="q"
        pending
        onValueChange={onValueChange}
      >
        <ChoiceOption
          multiple={false}
          value="b"
          selected={false}
          ordinal={2}
          label="Second"
          pending
        >
          Second
        </ChoiceOption>
      </ChoiceList>,
    );

    const option = screen.getByRole("radio", { name: "Second" });
    expect((option as HTMLButtonElement).disabled).toBe(false);
    expect(option.getAttribute("aria-disabled")).toBe("true");

    fireEvent.click(option);
    expect(onValueChange).not.toHaveBeenCalled();
  });
});
