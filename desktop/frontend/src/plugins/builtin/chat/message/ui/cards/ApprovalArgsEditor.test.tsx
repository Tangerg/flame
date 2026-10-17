import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { useApprovalArgsEditor } from "../../application/approvalArgsEditor";
import { ApprovalArgsEditor } from "./ApprovalArgsEditor";

function EditableArguments() {
  const editor = useApprovalArgsEditor({ originalArgs: '{"command":"pwd"}' });
  return (
    <ApprovalArgsEditor
      editing={editor.editing}
      argsText={editor.argsText}
      invalid={editor.invalid}
      onEditToggle={editor.setEditing}
      onTextChange={editor.setArgsText}
    />
  );
}

it("moves focus from Edit into the arguments and retains it while typing", () => {
  render(<EditableArguments />);
  const edit = screen.getByRole("button", { name: /^Edit$/ });
  edit.focus();
  fireEvent.click(edit);

  const field = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: /^Arguments$/,
  });
  expect(document.activeElement).toBe(field);
  expect(field.value).toBe('{"command":"pwd"}');

  fireEvent.change(field, { target: { value: '{"command":"ls"}' } });
  expect(field.value).toBe('{"command":"ls"}');
  expect(document.activeElement).toBe(field);
});
