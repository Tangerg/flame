import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { FormField, TextArea, TextField } from "../dist/index.js";

const stylesheet = readFileSync(
  resolve(import.meta.dirname, "../dist/styles.css"),
  "utf8",
);
let style: HTMLStyleElement;

beforeEach(() => {
  style = document.createElement("style");
  // Happy DOM needs normalized selector whitespace and a concrete host focus color.
  style.textContent = stylesheet
    .replace(/\s+/g, " ")
    .replaceAll("var(--color-focus-ring)", "#123456")
    .replaceAll("var(--color-negative)", "#bb1234");
  document.head.append(style);
});

it.each(["bare", "inline", "well"] as const)(
  "preserves %s validation alongside native visible focus",
  (variant) => {
    render(
      <div data-focus-inset="">
        <TextArea variant={variant} invalid aria-label="Configuration" />
      </div>,
    );
    const field = screen.getByLabelText("Configuration");
    act(() => field.focus());
    expect(getComputedStyle(field).outlineStyle).toBe("solid");
    expect(getComputedStyle(field).outlineOffset).toBe("1px");
    expect(getComputedStyle(field).boxShadow).toContain("#bb1234");
  },
);

afterEach(() => {
  cleanup();
  style.remove();
});

describe.each([
  ["single-line", TextField],
  ["multiline", TextArea],
] as const)("%s field focus", (_label, Control) => {
  it("keeps visible focus distinct from its validation state", () => {
    const view = render(
      <FormField label="Configuration">
        <Control invalid />
      </FormField>,
    );
    const field = screen.getByLabelText("Configuration");
    act(() => field.focus());
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(getComputedStyle(field).outlineStyle).toBe("solid");
    expect(
      Number.parseFloat(getComputedStyle(field).outlineWidth),
    ).toBeGreaterThanOrEqual(1.5);
    expect(getComputedStyle(field).outlineOffset).toBe("1px");

    view.rerender(
      <FormField label="Configuration">
        <Control />
      </FormField>,
    );
    act(() => field.focus());
    expect(field.getAttribute("aria-invalid")).toBeNull();
    expect(getComputedStyle(field).outlineStyle).toBe("solid");
  });

  it("keeps validation visible when focus moves to another control", () => {
    render(
      <>
        <FormField label="Configuration">
          <Control invalid />
        </FormField>
        <button type="button">Next field</button>
      </>,
    );
    const field = screen.getByLabelText("Configuration");
    const next = screen.getByRole("button", { name: "Next field" });
    act(() => field.focus());
    act(() => next.focus());

    expect(document.activeElement).toBe(next);
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(getComputedStyle(field).borderColor).toContain("#bb1234");
  });

  it("retains named native focus indication", () => {
    render(<Control aria-label="Configuration" invalid />);
    const field = screen.getByLabelText("Configuration");

    act(() => field.focus());

    expect(document.activeElement).toBe(field);
    expect(getComputedStyle(field).outlineStyle).toBe("solid");
  });
});
