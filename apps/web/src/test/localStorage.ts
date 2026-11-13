import { onTestFinished, vi } from "vitest";

export function mockLocalStorage() {
  const backing = window.localStorage;
  const storage = {
    get length() {
      return backing.length;
    },
    clear() {
      backing.clear();
    },
    getItem: vi.fn((key: string) => backing.getItem(key)),
    key(index: number) {
      return backing.key(index);
    },
    removeItem(key: string) {
      backing.removeItem(key);
    },
    setItem: vi.fn((key: string, value: string) => backing.setItem(key, value)),
  } satisfies Storage;
  const getter = vi.spyOn(window, "localStorage", "get").mockReturnValue(storage);
  const restore = () => getter.mockRestore();
  onTestFinished(restore);
  return { storage, backing, restore };
}
