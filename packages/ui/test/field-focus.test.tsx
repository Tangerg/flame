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
    .replaceAll("var(--color-focus-ring)", "#123456");
  document.head.append(style);
});

afterEach(() => {
  cleanup();
  style.remove();
  document.documentElement.removeAttribute("data-input-modality");
});

describe.each([
  ["single-line", TextField],
  ["multiline", TextArea],
] as const)("%s field focus", (_label, Control) => {
  it("keeps keyboard focus distinct from its validation state", () => {
    document.documentElement.setAttribute("data-input-modality", "keyboard");
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
    expect(getComputedStyle(field).outlineStyle || "none").toBe("none");
  });

  it("keeps pointer focus with the field's existing border", () => {
    document.documentElement.setAttribute("data-input-modality", "pointer");
    render(
      <FormField label="Configuration">
        <Control invalid />
      </FormField>,
    );
    const field = screen.getByLabelText("Configuration");
    act(() => field.focus());
    expect(getComputedStyle(field).outlineStyle || "none").toBe("none");
  });

  it("retains native focus indication without a host modality owner", () => {
    render(<Control aria-label="Configuration" invalid />);
    const field = screen.getByLabelText("Configuration");

    act(() => field.focus());

    expect(document.documentElement.hasAttribute("data-input-modality")).toBe(
      false,
    );
    expect(getComputedStyle(field).outlineStyle).toBe("solid");
  });
});
