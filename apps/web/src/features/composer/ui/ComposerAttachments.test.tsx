import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { useRef, useState } from "react";
import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ComposerAttachments } from "@/features/composer/ui/ComposerAttachments";
import { t } from "@/lib/i18n/index";
it("distinguishes referenced file types and still removes the selected reference", () => {
  const paths = ["src/Button.tsx", "assets/logo.png", "README.md", "package.json", "plan.pdf"];
  const value = paths.map((path) => `@${path}`).join(" ");
  const onChange = vi.fn();
  const view = testClient.render(
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
      composerInput={{ current: null }}
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

const REFERENCED = new Set(["README.md", "src/App.tsx"]);

function ReferenceDraft({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  const input = useRef<HTMLTextAreaElement>(null);
  return (
    <>
      <textarea
        aria-label="Draft"
        ref={input}
        value={value}
        onChange={(event) => setValue(event.target.value)}
      />
      <ComposerAttachments
        images={[]}
        pastes={[]}
        value={value}
        knownPaths={REFERENCED}
        onChange={setValue}
        onRemoveImage={vi.fn()}
        onRemovePaste={vi.fn()}
        onEditPaste={vi.fn()}
        onRestorePaste={vi.fn()}
        composerInput={input}
      />
    </>
  );
}

it("returns to the same input after removing a referenced file", async () => {
  testClient.render(<ReferenceDraft initial="Check @README.md and @src/App.tsx please" />);
  const input = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Draft" });
  const remove = screen.getAllByRole("button", { name: t("common.remove") })[0]!;
  remove.focus();
  fireEvent.click(remove);

  expect(document.activeElement).toBe(input);
  expect(screen.getByRole("textbox", { name: "Draft" })).toBe(input);
  expect(input.value).toBe("Check and @src/App.tsx please");
  expect(await screen.findByText("App.tsx")).toBeTruthy();
});

it("keeps focus in place when reference removal has no current input", () => {
  const onChange = vi.fn();
  testClient.render(
    <ComposerAttachments
      images={[]}
      pastes={[]}
      value="@README.md @src/App.tsx"
      knownPaths={REFERENCED}
      onChange={onChange}
      onRemoveImage={vi.fn()}
      onRemovePaste={vi.fn()}
      onEditPaste={vi.fn()}
      onRestorePaste={vi.fn()}
      composerInput={{ current: null }}
    />,
  );
  const remove = screen.getAllByRole("button", { name: t("common.remove") })[1]!;
  remove.focus();
  fireEvent.click(remove);

  expect(onChange).toHaveBeenCalledOnce();
  expect(onChange).toHaveBeenCalledWith("@README.md");
  expect(document.activeElement).toBe(remove);
});
