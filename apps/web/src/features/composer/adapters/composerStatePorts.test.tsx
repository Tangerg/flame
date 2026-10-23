import { useEffect, useRef } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useComposerStore } from "@/features/composer/adapters/composerStore";
import {
  beginComposerDraftReplacement,
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import { registerComposerFocusTarget } from "@/features/composer/public/focus";

function ComposerField() {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const field = useRef<HTMLTextAreaElement>(null);
  useEffect(() => registerComposerFocusTarget(field.current), []);
  return (
    <textarea
      ref={field}
      aria-label="Message"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

function Harness({ fieldKey = "original" }: { fieldKey?: string }) {
  return (
    <>
      <ComposerField key={fieldKey} />
      <button type="button">Other task</button>
    </>
  );
}

function beginReplacement() {
  let replace!: ReturnType<typeof beginComposerDraftReplacement>;
  act(() => {
    replace = beginComposerDraftReplacement();
  });
  return replace;
}

beforeEach(() => {
  useComposerStore.getState().loadSession("focus-draft");
  useComposerStore.getState().replaceDraft({ text: "old" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("draft replacement focus", () => {
  it("enters the Composer on intent and places the completed input's caret at the end", () => {
    render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const replace = beginReplacement();
    expect(document.activeElement).toBe(field);
    expect(field.value).toBe("old");

    let applied = false;
    act(() => {
      applied = replace({ text: "longer canonical input" });
    });

    expect(applied).toBe(true);
    expect(field.value).toBe("longer canonical input");
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
  });

  it("keeps another control's later focus when the unchanged draft is replaced", () => {
    render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const replace = beginReplacement();
    const other = screen.getByRole("button", { name: "Other task" });
    act(() => other.focus());
    const select = vi.spyOn(field, "setSelectionRange");

    act(() => {
      replace({ text: "canonical input" });
    });

    expect(field.value).toBe("canonical input");
    expect(document.activeElement).toBe(other);
    expect(select).not.toHaveBeenCalled();
  });

  it("does not move the selection of a newly registered focus target", () => {
    const { rerender } = render(<Harness />);
    const replace = beginReplacement();
    rerender(<Harness fieldKey="successor" />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    act(() => field.focus());
    const select = vi.spyOn(field, "setSelectionRange");

    act(() => {
      replace({ text: "canonical input" });
    });

    expect(field.value).toBe("canonical input");
    expect(document.activeElement).toBe(field);
    expect(select).not.toHaveBeenCalled();
  });

  it("preserves a later draft and its caret when replacement is refused", () => {
    render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const replace = beginReplacement();
    fireEvent.change(field, { target: { value: "later authored input" } });
    field.setSelectionRange(3, 3);
    const select = vi.spyOn(field, "setSelectionRange");
    let applied = true;

    act(() => {
      applied = replace({ text: "canonical input" });
    });

    expect(applied).toBe(false);
    expect(field.value).toBe("later authored input");
    expect(field.selectionStart).toBe(3);
    expect(field.selectionEnd).toBe(3);
    expect(document.activeElement).toBe(field);
    expect(select).not.toHaveBeenCalled();
  });

  it("retains the synchronous replacement's immediate focus behavior", () => {
    render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    act(() => screen.getByRole("button", { name: "Other task" }).focus());

    act(() => {
      replaceComposerDraft({ text: "synchronous edit" });
    });

    expect(field.value).toBe("synchronous edit");
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
  });
});
