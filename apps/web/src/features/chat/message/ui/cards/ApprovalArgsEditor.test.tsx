import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { useApprovalArgsEditor } from "@/features/chat/message/application/approvalArgsEditor";
import { ApprovalArgsEditor } from "@/features/chat/message/ui/cards/ApprovalArgsEditor";
function EditableArguments({ disabled = false }: { disabled?: boolean }) {
  const editor = useApprovalArgsEditor({ originalArgs: '{"command":"pwd"}' });
  return (
    <ApprovalArgsEditor
      editing={editor.editing}
      argsText={editor.argsText}
      invalid={editor.invalid}
      disabled={disabled}
      onEditToggle={editor.setEditing}
      onTextChange={editor.setArgsText}
    />
  );
}

function argumentsField() {
  return screen.getByRole<HTMLTextAreaElement>("textbox", { name: /^Arguments$/ });
}

it("moves focus from Edit into the arguments and retains it while typing", () => {
  render(<EditableArguments />);
  const edit = screen.getByRole("button", { name: /^Edit$/ });
  edit.focus();
  fireEvent.click(edit);

  const field = argumentsField();
  expect(document.activeElement).toBe(field);
  expect(field.value).toBe('{"command":"pwd"}');
  expect(field.readOnly).toBe(false);
  expect(field.getAttribute("aria-disabled")).toBeNull();

  fireEvent.change(field, { target: { value: '{"command":"ls"}' } });
  expect(field.value).toBe('{"command":"ls"}');
  expect(document.activeElement).toBe(field);
});

it("keeps a blocked Edit focusable while refusing to open the arguments", () => {
  render(<EditableArguments disabled />);
  const edit = screen.getByRole<HTMLButtonElement>("button", { name: /^Edit$/ });
  expect(edit.getAttribute("aria-disabled")).toBe("true");
  expect(edit.disabled).toBe(false);

  edit.focus();
  expect(document.activeElement).toBe(edit);
  fireEvent.click(edit);
  fireEvent.keyDown(edit, { key: "Enter" });
  fireEvent.keyDown(edit, { key: " " });
  fireEvent.keyUp(edit, { key: " " });

  expect(screen.queryByRole("textbox")).toBeNull();
  expect(document.activeElement).toBe(edit);
});

it("makes an open field read-only but focusable while blocked and restores the same draft", () => {
  const { rerender } = render(<EditableArguments />);
  fireEvent.click(screen.getByRole("button", { name: /^Edit$/ }));
  fireEvent.change(argumentsField(), { target: { value: '{"command":"ls"}' } });

  rerender(<EditableArguments disabled />);
  const blocked = argumentsField();
  expect(blocked.readOnly).toBe(true);
  expect(blocked.getAttribute("aria-disabled")).toBe("true");
  expect(blocked.disabled).toBe(false);
  expect(blocked.value).toBe('{"command":"ls"}');
  blocked.focus();
  expect(document.activeElement).toBe(blocked);

  rerender(<EditableArguments />);
  const restored = argumentsField();
  expect(restored).toBe(blocked);
  expect(restored.readOnly).toBe(false);
  expect(restored.getAttribute("aria-disabled")).toBeNull();
  expect(restored.value).toBe('{"command":"ls"}');
  expect(document.activeElement).toBe(restored);
});
