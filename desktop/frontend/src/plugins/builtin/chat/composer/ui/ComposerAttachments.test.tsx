import { fireEvent, render } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ComposerAttachments } from "./ComposerAttachments";

it("distinguishes referenced file types and still removes the selected reference", () => {
  const paths = ["src/Button.tsx", "assets/logo.png", "README.md", "package.json", "plan.pdf"];
  const value = paths.map((path) => `@${path}`).join(" ");
  const onChange = vi.fn();
  const view = render(
    <ComposerAttachments
      images={[]}
      pastes={[]}
      value={value}
      knownPaths={new Set(paths)}
      onChange={onChange}
      onRemoveImage={vi.fn()}
      onRemovePaste={vi.fn()}
      onEditPaste={vi.fn()}
      onRestorePaste={vi.fn()}
    />,
  );
  const chips = [...view.container.querySelectorAll('[data-slot="chip"]')];
  expect(chips.map((chip) => chip.querySelector("svg")?.getAttribute("data-icon-name"))).toEqual([
    "file-code",
    "file-image",
    "file-text",
    "file-braces",
    "file-pdf",
  ]);
  fireEvent.click(chips[1]!.querySelector("button")!);
  expect(onChange).toHaveBeenCalledWith("@src/Button.tsx @README.md @package.json @plan.pdf");
});
