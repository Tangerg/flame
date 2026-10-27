import { onTestFinished, vi } from "vitest";

export function mockSelectLayout() {
  // Happy DOM has no layout. Shared UI and native acceptance cover actual anchor clipping.
  const originalRect = HTMLElement.prototype.getBoundingClientRect;
  const rect = vi
    .spyOn(HTMLElement.prototype, "getBoundingClientRect")
    .mockImplementation(function (this: HTMLElement) {
      if (this === document.documentElement || this === document.body)
        return new DOMRect(0, 0, 1024, 768);
      if (this.getAttribute("role") !== "combobox") return originalRect.call(this);
      if (this.closest("[hidden], [inert]")) return new DOMRect();
      return new DOMRect(20, 20, 160, 32);
    });
  const width = vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(1024);
  const height = vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(768);
  onTestFinished(() => {
    rect.mockRestore();
    width.mockRestore();
    height.mockRestore();
  });
}

export function mockSelectViewport(viewport: HTMLElement) {
  vi.spyOn(viewport, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 1024, 400));
  vi.spyOn(viewport, "clientWidth", "get").mockReturnValue(1024);
  vi.spyOn(viewport, "clientHeight", "get").mockReturnValue(400);
}
