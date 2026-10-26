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
import { ChoiceList, ChoiceOption } from "@/ui/atoms/choice-list";
import { TextArea, TextField } from "@flame/ui/text-field";
describe("field pending", () => {
  it("keeps a text field focusable and unwritable while its action is in flight", () => {
    testClient.render(<TextField aria-label="Path" defaultValue="/repo" pending />);
    const field = screen.getByRole("textbox", { name: "Path" }) as HTMLInputElement;

    expect(field.disabled).toBe(false);
    expect(field.readOnly).toBe(true);
    expect(field.getAttribute("aria-disabled")).toBe("true");

    field.focus();
    expect(document.activeElement).toBe(field);
  });

  it("keeps a textarea focusable and unwritable while its action is in flight", () => {
    testClient.render(<TextArea aria-label="Note" defaultValue="hello" pending />);
    const area = screen.getByRole("textbox", { name: "Note" }) as HTMLTextAreaElement;

    expect(area.disabled).toBe(false);
    expect(area.readOnly).toBe(true);
    expect(area.getAttribute("aria-disabled")).toBe("true");
  });

  it("writes normally when it is not pending", () => {
    testClient.render(<TextField aria-label="Path" />);
    const field = screen.getByRole("textbox", { name: "Path" }) as HTMLInputElement;
    expect(field.readOnly).toBe(false);
    expect(field.getAttribute("aria-disabled")).toBeNull();
  });

  it.each([{ value: ["a"] }, { value: [] }])(
    "keeps a pending choice list immutable with initial $value",
    ({ value }) => {
      const onValueChange = vi.fn();
      testClient.render(
        <ChoiceList
          multiple={false}
          value={value}
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
      expect(option.getAttribute("aria-checked")).toBe("false");
    },
  );
});
