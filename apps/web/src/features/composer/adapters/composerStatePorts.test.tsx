import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { useEffect, useRef } from "react";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beginComposerDraftReplacement,
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import {
  captureComposerFocus,
  focusComposer,
  registerComposerFocusPresentation,
  registerComposerFocusTarget,
} from "@/features/composer/public/focus";

function ComposerField() {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const field = useRef<HTMLTextAreaElement>(null);
  useEffect(() => registerComposerFocusTarget(testClient.composer.access, field.current), []);
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
    replace = beginComposerDraftReplacement(testClient.composer.access);
  });
  return replace;
}

beforeEach(() => {
  testClient.composer.store.getState().loadSession("focus-draft");
  testClient.composer.store.getState().replaceDraft({ text: "old" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("draft replacement focus", () => {
  it("enters the Composer on intent and places the completed input's caret at the end", () => {
    testClient.render(<Harness />);
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
    testClient.render(<Harness />);
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
    const { rerender } = testClient.render(<Harness />);
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
    testClient.render(<Harness />);
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
    testClient.render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    act(() => screen.getByRole("button", { name: "Other task" }).focus());

    act(() => {
      replaceComposerDraft(testClient.composer.access, { text: "synchronous edit" });
    });

    expect(field.value).toBe("synchronous edit");
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
  });
});

describe("draft focus presentation", () => {
  it("does not unregister a newer presentation when an older owner cleans up", () => {
    testClient.render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const previous = vi.fn();
    const current = vi.fn();
    const unregisterPrevious = registerComposerFocusPresentation(
      testClient.composer.access,
      previous,
    );
    const unregisterCurrent = registerComposerFocusPresentation(
      testClient.composer.access,
      current,
    );
    unregisterPrevious();
    act(() => screen.getByRole("button", { name: "Other task" }).focus());

    act(() =>
      replaceComposerDraft(testClient.composer.access, { text: "Current presentation's draft" }),
    );

    expect(previous).not.toHaveBeenCalled();
    expect(current).toHaveBeenCalledTimes(1);
    expect(document.activeElement).not.toBe(field);
    unregisterCurrent();
    act(() =>
      replaceComposerDraft(testClient.composer.access, { text: "Locally presented draft" }),
    );
    expect(current).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(field.value.length);
  });

  it("delivers the captured target and caret after presentation", () => {
    testClient.render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const other = screen.getByRole("button", { name: "Other task" });
    act(() => other.focus());
    let deliver!: () => void;
    const present = vi.fn((focus: () => void) => {
      deliver = focus;
    });
    const unregister = registerComposerFocusPresentation(testClient.composer.access, present);

    act(() =>
      replaceComposerDraft(testClient.composer.access, {
        text: "Edited input awaiting presentation",
      }),
    );

    expect(present).toHaveBeenCalledTimes(1);
    expect(field.value).toBe("Edited input awaiting presentation");
    expect(document.activeElement).toBe(other);
    act(() => deliver());
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    unregister();
  });

  it.each(["remounted", "unregistered"])(
    "retires the captured delivery when its input is %s",
    (retirement) => {
      const view = testClient.render(<Harness />);
      const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
      const unregisterTarget = registerComposerFocusTarget(testClient.composer.access, field);
      let deliver!: () => void;
      const unregisterPresentation = registerComposerFocusPresentation(
        testClient.composer.access,
        (focus) => {
          deliver = focus;
        },
      );
      act(() => replaceComposerDraft(testClient.composer.access, { text: "Edited input" }));

      if (retirement === "remounted") view.rerender(<Harness fieldKey="successor" />);
      else unregisterTarget();
      const currentField = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
      currentField.setSelectionRange(2, 2);
      const other = screen.getByRole("button", { name: "Other task" });
      act(() => other.focus());
      act(() => deliver());

      expect(document.activeElement).toBe(other);
      expect(currentField.selectionStart).toBe(2);
      expect(currentField.selectionEnd).toBe(2);
      unregisterTarget();
      unregisterPresentation();
    },
  );

  it("keeps low-level and captured focus local when presentation is registered", () => {
    testClient.render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const other = screen.getByRole("button", { name: "Other task" });
    const present = vi.fn();
    const unregister = registerComposerFocusPresentation(testClient.composer.access, present);
    act(() => other.focus());

    act(() => focusComposer(testClient.composer.access, 1));

    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(1);
    act(() => other.focus());
    const restore = captureComposerFocus(testClient.composer.access);
    act(() => restore());
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(1);
    expect(present).not.toHaveBeenCalled();
    unregister();
  });
});
