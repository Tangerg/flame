import { createTheme, type Theme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { CustomThemeColors } from "@/features/theme/public/preferences/CustomThemeColors";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { formatHex } from "@/lib/color";

let owner: Theme;
const saved = { bg: "#f2ede4", fg: "#202020" };
const defaults = { bg: "#0f1117", fg: "#e6e8ee" };
const nativePicker = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "showPicker");

beforeEach(() => {
  owner = createTheme(document, window);
  owner.store.setState({ theme: "custom", customTheme: { ...saved } });
});

afterEach(() => {
  cleanup();
  owner.dispose();
  if (nativePicker) Object.defineProperty(HTMLInputElement.prototype, "showPicker", nativePicker);
  else Reflect.deleteProperty(HTMLInputElement.prototype, "showPicker");
});

function mount() {
  return render(
    <ThemeProvider value={owner.access}>
      <CustomThemeColors />
    </ThemeProvider>,
  );
}

function choose(label: string, value: string) {
  const show = vi.fn(function (this: HTMLInputElement) {
    fireEvent.change(this, { target: { value } });
  });
  Object.defineProperty(HTMLInputElement.prototype, "showPicker", {
    configurable: true,
    value: show,
  });
  fireEvent.click(screen.getByRole("button", { name: label }));
  expect(show).toHaveBeenCalledOnce();
}

function expectColor(label: string, value: string) {
  expect(
    screen.getByRole("button", { name: label, description: new RegExp(value, "i") }),
  ).toBeTruthy();
}

it("previews a low-contrast pair without changing the applied colors", () => {
  mount();
  choose("Background", saved.fg);
  expect(owner.store.getState().customTheme).toEqual(saved);
  expect(
    formatHex(screen.getByRole("figure", { name: "Color preview" }).style.backgroundColor),
  ).toBe(saved.fg);
  expect(screen.getByRole("status").textContent).toContain("1.00:1");
  expect(screen.getByRole("status").textContent).toContain("4.5:1");
  const apply = screen.getByRole("button", { name: "Apply colors" });
  expect(apply.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(apply);
  expect(owner.store.getState().customTheme).toEqual(saved);
});

it("applies both explicit colors in one update and retains action focus", () => {
  mount();
  const updates: unknown[] = [];
  const stop = owner.store.subscribe((next) => updates.push(next.customTheme));
  choose("Background", "#161616");
  choose("Foreground", "#eeeeee");
  expect(updates).toEqual([]);
  const apply = screen.getByRole<HTMLButtonElement>("button", { name: "Apply colors" });
  apply.focus();
  fireEvent.click(apply);
  expect(updates).toEqual([{ bg: "#161616", fg: "#eeeeee" }]);
  expect(owner.store.getState().theme).toBe("custom");
  expect(document.activeElement).toBe(apply);
  expect(apply.disabled).toBe(false);
  expect(apply.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(apply);
  expect(updates).toHaveLength(1);
  stop();
});

it("discards the preview to the latest applied pair without mutating it", () => {
  mount();
  choose("Background", "#161616");
  const external = { bg: "#eee7db", fg: "#232323" };
  act(() => owner.store.getState().setCustomTheme(external));
  const discard = screen.getByRole("button", { name: "Discard changes" });
  discard.focus();
  fireEvent.click(discard);
  expectColor("Background", external.bg);
  expectColor("Foreground", external.fg);
  expect(owner.store.getState().customTheme).toEqual(external);
  expect(document.activeElement).toBe(discard);
  expect(discard.getAttribute("aria-disabled")).toBe("true");
});

it("restores only the color pair and discards its unfinished preview", () => {
  owner.store.setState({
    accent: "#7c5cfc",
    fontSize: 18,
    contrast: 100,
    paperTexture: "cotton",
    paperTextureStrength: 5,
  });
  const before = owner.store.getState();
  mount();
  choose("Background", "#161616");
  const restore = screen.getByRole("button", { name: "Restore default colors" });
  restore.focus();
  fireEvent.click(restore);
  expect(owner.store.getState()).toEqual({ ...before, customTheme: defaults });
  expectColor("Background", defaults.bg);
  expectColor("Foreground", defaults.fg);
  expect(document.activeElement).toBe(restore);
  expect(screen.getByRole("button", { name: "Apply colors" }).getAttribute("aria-disabled")).toBe(
    "true",
  );
});

it("keeps an already unreadable saved pair until explicit recovery", () => {
  owner.store.setState({ customTheme: { bg: "#808080", fg: "#808080" } });
  mount();
  expect(owner.store.getState().customTheme).toEqual({ bg: "#808080", fg: "#808080" });
  const restore = screen.getByRole("button", { name: "Restore default colors" });
  expect(restore.getAttribute("data-variant")).toBe("primary");
  restore.focus();
  fireEvent.click(restore);
  expect(owner.store.getState().customTheme).toEqual(defaults);
  expect(document.activeElement).toBe(restore);
});

it("checks the actual reading planes rather than the canvas pair alone", () => {
  mount();
  choose("Background", "#ffffff");
  choose("Foreground", "#767676");
  expect(screen.getByRole("status").textContent).toContain("4.5:1");
  expect(screen.getByRole("button", { name: "Apply colors" }).getAttribute("aria-disabled")).toBe(
    "true",
  );
  expect(owner.store.getState().customTheme).toEqual(saved);
});

it("rechecks a retained preview when paper changes its painted reading planes", () => {
  mount();
  choose("Background", "#ffffff");
  choose("Foreground", "#686868");
  const apply = screen.getByRole("button", { name: "Apply colors" });
  expect(apply.getAttribute("aria-disabled")).not.toBe("true");
  act(() => {
    owner.store.getState().setPaperTexture("xuan");
    owner.store.getState().setPaperTextureStrength(5);
  });
  expect(apply.getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByRole("status").textContent).toContain("4.5:1");
  expectColor("Foreground", "#686868");
  expect(owner.store.getState().customTheme).toEqual(saved);
});

it("does not discard a color preview when an independent preference changes", () => {
  mount();
  choose("Foreground", "#171717");
  act(() => owner.store.getState().setAccent("#7c5cfc"));
  expectColor("Foreground", "#171717");
  fireEvent.click(screen.getByRole("button", { name: "Apply colors" }));
  expect(owner.store.getState()).toMatchObject({
    customTheme: { ...saved, fg: "#171717" },
    accent: "#7c5cfc",
  });
});

it("ends the preview when a different theme is selected", () => {
  mount();
  choose("Foreground", "#171717");
  act(() => owner.store.getState().setTheme("light"));
  expect(screen.queryByRole("button", { name: "Foreground" })).toBeNull();
  act(() => owner.store.getState().setTheme("custom"));
  expectColor("Foreground", saved.fg);
  expect(owner.store.getState().customTheme).toEqual(saved);
});
