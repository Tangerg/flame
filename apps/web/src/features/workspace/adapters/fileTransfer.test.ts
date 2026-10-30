import { afterEach, describe, expect, it, vi } from "vitest";
import { pickTextFile } from "@/features/workspace/adapters/fileTransfer";
afterEach(() => vi.restoreAllMocks());

describe("fileTransfer", () => {
  it("settles native picker cancellation as an empty selection", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});

    const selection = pickTextFile("application/json,.json");
    const observed = observe(selection);
    const picker = click.mock.instances[0] as HTMLInputElement;
    expect(picker).toBeDefined();

    picker.dispatchEvent(new Event("cancel"));
    await drainMicrotasks();
    const settledAtCancel = observed.settled();

    picker.dispatchEvent(new Event("change"));
    await expect(selection).resolves.toBeNull();
    expect(settledAtCancel).toBe(true);
    expect(observed.value()).toBeNull();
  });

  it("settles a change without a selected file as an empty selection", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});

    const selection = pickTextFile(".json");
    choose(click.mock.instances[0] as HTMLInputElement, null);

    await expect(selection).resolves.toBeNull();
  });

  it.each([
    ["Unicode", '{"title":"Résumé 🔥"}\n'],
    ["empty", ""],
  ])("resolves the exact %s text of the selected file", async (_kind, content) => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});

    const selection = pickTextFile(".json");
    choose(click.mock.instances[0] as HTMLInputElement, new File([content], "chosen.json"));

    await expect(selection).resolves.toBe(content);
  });

  it("surfaces a failed read of the selected file and releases the picker", async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    const failure = new DOMException("The I/O read operation failed.", "NotReadableError");
    const unreadable = new File(["{}"], "unreadable.json");
    const read = vi.spyOn(unreadable, "text").mockRejectedValue(failure);

    const failed = pickTextFile(".json");
    const picker = click.mock.instances[0] as HTMLInputElement;
    choose(picker, unreadable);
    await expect(failed).rejects.toBe(failure);

    picker.dispatchEvent(new Event("cancel"));
    picker.dispatchEvent(new Event("change"));
    await drainMicrotasks();
    expect(read).toHaveBeenCalledOnce();

    const next = pickTextFile(".json");
    const nextPicker = click.mock.instances[1] as HTMLInputElement;
    expect(nextPicker).toBeDefined();
    expect(nextPicker).not.toBe(picker);
    choose(nextPicker, new File(['{"ok":true}'], "readable.json"));
    await expect(next).resolves.toBe('{"ok":true}');
  });

  it.each([
    ["completion", (read: PromiseWithResolvers<string>) => read.resolve('{"late":true}')],
    ["failure", (read: PromiseWithResolvers<string>) => read.reject(new Error("late read"))],
  ])("keeps a cancelled selection empty after a late read %s", async (_outcome, finish) => {
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    const read = Promise.withResolvers<string>();
    const file = new File(["{}"], "slow.json");
    vi.spyOn(file, "text").mockReturnValue(read.promise);

    const selection = pickTextFile(".json");
    const observed = observe(selection);
    const picker = click.mock.instances[0] as HTMLInputElement;
    choose(picker, file);
    picker.dispatchEvent(new Event("cancel"));
    await drainMicrotasks();
    const settledAtCancel = observed.settled();

    finish(read);
    await drainMicrotasks();

    expect(settledAtCancel).toBe(true);
    await expect(selection).resolves.toBeNull();
  });
});

function choose(picker: HTMLInputElement, file: File | null): void {
  Object.defineProperty(picker, "files", { configurable: true, value: file ? [file] : [] });
  picker.dispatchEvent(new Event("change"));
}

function observe<T>(operation: Promise<T>): {
  settled(): boolean;
  value(): T | undefined;
} {
  let settled = false;
  let value: T | undefined;
  void operation.then(
    (result) => {
      value = result;
      settled = true;
    },
    () => {
      settled = true;
    },
  );
  return {
    settled: () => settled,
    value: () => value,
  };
}

async function drainMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}
