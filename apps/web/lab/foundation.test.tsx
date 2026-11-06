import { type ReactElement } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { FontScaleProvider, TooltipProvider } from "@flame/ui";
import {
  ButtonExample,
  CheckboxExample,
  SearchExample,
  TextFieldExample,
} from "./foundationControls";
import { FormExample } from "./foundationComposition";
import { DisclosureExample, ModalExample } from "./foundationOverlays";
import { foundationViews as views } from "./foundationVariants";

afterEach(cleanup);

function renderExample(example: ReactElement) {
  return render(
    <FontScaleProvider fontSize={14}>
      <TooltipProvider>{example}</TooltipProvider>
    </FontScaleProvider>,
  );
}

it("records real checkbox changes while a disabled example cannot originate a change", () => {
  const observe = vi.fn();
  const view = renderExample(
    <CheckboxExample variant={views.editable.id} locale="en" observe={observe} />,
  );
  const checkbox = screen.getByRole("checkbox", { name: "Include details" });
  // happy-dom activates a label before React cancels its mark's native click.
  fireEvent.click(checkbox.closest("label")!);
  expect(checkbox.getAttribute("aria-checked")).toBe("true");
  expect(observe).toHaveBeenLastCalledWith("checkedChange", true);
  view.unmount();
  observe.mockClear();

  renderExample(<CheckboxExample variant={views.disabled.id} locale="en" observe={observe} />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Include details" }));
  expect(observe).not.toHaveBeenCalled();
});

it("keeps disabled and pending buttons at the production activation boundary", () => {
  const observe = vi.fn();
  renderExample(<ButtonExample variant={views.states.id} locale="en" observe={observe} />);
  const buttons = screen.getAllByRole("button", { name: "Continue" });
  for (const button of buttons) fireEvent.click(button);
  expect(observe.mock.calls).toEqual([["click", views.editable.id]]);
  expect(buttons[1]!.hasAttribute("disabled")).toBe(true);
  expect(buttons[2]!.getAttribute("aria-disabled")).toBe("true");
});

it("filters local examples and clears through the shared search action without losing focus", () => {
  const observe = vi.fn();
  renderExample(<SearchExample variant={views.editable.id} locale="en" observe={observe} />);
  const search = screen.getByRole("searchbox", { name: "Search examples" });
  fireEvent.change(search, { target: { value: "metr" } });
  expect(screen.getAllByRole("listitem").map((item) => item.textContent)).toEqual(["Metrics"]);
  expect(observe).toHaveBeenLastCalledWith("queryChange.length", "metr".length);
  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expect((search as HTMLInputElement).value).toBe("");
  expect(screen.getAllByRole("listitem")).toHaveLength(3);
  expect(document.activeElement).toBe(search);
  expect(observe).toHaveBeenLastCalledWith("queryChange.length", 0);
});

it("clears validation after editing and keeps comparison fields on the same draft", () => {
  const observe = vi.fn();
  renderExample(<TextFieldExample variant={views.states.id} locale="en" observe={observe} />);
  const name = screen.getByRole("textbox", { name: "Name" });
  expect(name.getAttribute("aria-invalid")).toBe("true");
  fireEvent.change(name, { target: { value: "A local example" } });
  expect(name.hasAttribute("aria-invalid")).toBe(false);
  expect(screen.queryByText("Enter a nonempty name.")).toBeNull();
  for (const field of screen.getAllByRole("textbox")) {
    expect((field as HTMLInputElement).value).toBe("A local example");
  }
  expect(observe).toHaveBeenLastCalledWith("valueChange.length", "A local example".length);
});

it("reports validation instead of a successful form submission and preserves literal draft input", () => {
  const observe = vi.fn();
  renderExample(<FormExample variant={views.editable.id} locale="en" observe={observe} />);
  fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
  expect(screen.getByRole("alert").textContent).toBe("Enter a nonempty name.");
  expect(observe).toHaveBeenLastCalledWith("validation", "name");
  expect(screen.queryByRole("status")).toBeNull();

  fireEvent.change(screen.getByRole("textbox", { name: "Name" }), {
    target: { value: "  Local draft  " },
  });
  expect(screen.queryByRole("alert")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
  expect(observe).toHaveBeenLastCalledWith("submit.length", "  Local draft  ".length);
  expect((screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement).value).toBe(
    "  Local draft  ",
  );
  expect(JSON.stringify(observe.mock.calls)).not.toContain("Local draft");
  expect(screen.getByRole("status").textContent).toBe("The local draft was saved.");
});

it("saves through the real dialog and returns focus to its captured opener", async () => {
  const observe = vi.fn();
  renderExample(<ModalExample variant={views.md.id} locale="en" observe={observe} />);
  const opener = screen.getByRole("button", { name: "Open dialog" });
  fireEvent.click(opener);
  const dialog = await screen.findByRole("dialog", { name: "Local component preview" });
  fireEvent.change(within(dialog).getByRole("textbox", { name: "Note" }), {
    target: { value: "Keep this local note" },
  });
  fireEvent.click(within(dialog).getByRole("button", { name: "Save draft" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(observe).toHaveBeenCalledWith("save.length", "Keep this local note".length);
  expect(JSON.stringify(observe.mock.calls)).not.toContain("Keep this local note");
  expect(screen.getByRole("status").textContent).toBe("The local draft was saved.");
  await waitFor(() => expect(document.activeElement).toBe(opener));
});

it("keeps the nested draft when a retained disclosure closes and reopens", async () => {
  const observe = vi.fn();
  renderExample(<DisclosureExample variant={views.retained.id} locale="en" observe={observe} />);
  const trigger = screen.getByRole("button", { name: "Recorded details" });
  fireEvent.click(trigger);
  const input = await screen.findByRole("textbox", { name: "Retained draft" });
  fireEvent.change(input, { target: { value: "Preserved after collapse" } });
  fireEvent.click(trigger);
  await waitFor(() => expect(trigger.getAttribute("aria-expanded")).toBe("false"));
  fireEvent.click(trigger);
  expect(
    ((await screen.findByRole("textbox", { name: "Retained draft" })) as HTMLInputElement).value,
  ).toBe("Preserved after collapse");
  expect(observe).toHaveBeenLastCalledWith("openChange", true);
});
