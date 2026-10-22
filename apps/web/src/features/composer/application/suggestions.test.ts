import { describe, it, expect, vi } from "vitest";
import {
  handleSuggestionKey,
  type SuggestionList,
} from "@/features/composer/application/suggestions";

function list<T>(over: Partial<SuggestionList<T>>): SuggestionList<T> {
  return {
    open: true,
    items: [],
    index: 0,
    setIndex: vi.fn(),
    accept: vi.fn(),
    dismiss: vi.fn(),
    ...over,
  };
}

const press = (key: string, shiftKey = false) => ({ key, shiftKey });

describe("handleSuggestionKey", () => {
  it("ignores every key while closed", () => {
    const l = list({ open: false, items: ["a"] });
    for (const key of ["ArrowUp", "ArrowDown", "Enter", "Tab", "Escape"])
      expect(handleSuggestionKey(l, press(key))).toBe(false);
  });

  describe("open with no items (loading / error / no matches)", () => {
    it("consumes arrows so they cannot fall through to history recall", () => {
      const l = list({ items: [] });
      expect(handleSuggestionKey(l, press("ArrowUp"))).toBe(true);
      expect(handleSuggestionKey(l, press("ArrowDown"))).toBe(true);
      expect(l.setIndex).not.toHaveBeenCalled();
      expect(l.dismiss).not.toHaveBeenCalled();
    });

    it("dismisses on Enter instead of sending the raw token", () => {
      const l = list({ items: [] });
      expect(handleSuggestionKey(l, press("Enter"))).toBe(true);
      expect(l.dismiss).toHaveBeenCalledOnce();
      expect(l.accept).not.toHaveBeenCalled();
    });

    it("dismisses on Tab instead of moving focus out", () => {
      const l = list({ items: [] });
      expect(handleSuggestionKey(l, press("Tab"))).toBe(true);
      expect(l.dismiss).toHaveBeenCalledOnce();
    });

    it("lets Shift+Enter through for a newline", () => {
      const l = list({ items: [] });
      expect(handleSuggestionKey(l, press("Enter", true))).toBe(false);
      expect(l.dismiss).not.toHaveBeenCalled();
    });

    it("dismisses on Escape", () => {
      const l = list({ items: [] });
      expect(handleSuggestionKey(l, press("Escape"))).toBe(true);
      expect(l.dismiss).toHaveBeenCalledOnce();
    });
  });

  describe("open with items", () => {
    it("moves the selection with arrows, wrapping", () => {
      const l = list({ items: ["a", "b", "c"], index: 2 });
      expect(handleSuggestionKey(l, press("ArrowDown"))).toBe(true);
      expect(l.setIndex).toHaveBeenCalledWith(0);
      expect(handleSuggestionKey(l, press("ArrowUp"))).toBe(true);
      expect(l.setIndex).toHaveBeenCalledWith(1);
    });

    it("accepts the selected item on Enter and Tab", () => {
      const l = list({ items: ["a", "b"], index: 1 });
      expect(handleSuggestionKey(l, press("Enter"))).toBe(true);
      expect(l.accept).toHaveBeenCalledWith("b");
      expect(handleSuggestionKey(l, press("Tab"))).toBe(true);
      expect(l.accept).toHaveBeenCalledWith("b");
    });
  });
});
