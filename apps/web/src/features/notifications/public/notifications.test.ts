import { describe, expect, it, vi } from "vitest";
import { NotificationFeed } from "@/features/notifications/public/notifications";

describe("window notification history", () => {
  it("issues exact identities and dismisses only the selected entry", () => {
    const messages = new NotificationFeed(() => {});
    const first = messages.record({ plugin: "p", level: "info", message: "first" })!;
    const second = messages.record({ plugin: "p", level: "info", message: "second" })!;
    expect(first.id).toMatch(/^\d+$/);
    expect(BigInt(second.id)).toBe(BigInt(first.id) + 1n);
    messages.dismiss(first.id);
    expect(messages.snapshot()).toEqual([{ ...first, dismissed: true }, second]);
  });

  it("keeps history, transient presentation and separate windows independent", () => {
    const present = vi.fn();
    const first = new NotificationFeed(present);
    const second = new NotificationFeed(() => {});
    first.notify("error", "Could not save", { source: "session", description: "Read the details" });
    expect(first.snapshot()).toMatchObject([
      { plugin: "session", message: "Could not save — Read the details" },
    ]);
    expect(second.snapshot()).toEqual([]);
    first.show({ level: "success", message: "Copied" });
    expect(first.snapshot()).toHaveLength(1);
    first.clearAll();
    expect(first.snapshot()).toEqual([]);
    expect(present).toHaveBeenCalledTimes(2);
  });

  it("retires late publications, history mutations and captured actions with its window", () => {
    const present = vi.fn();
    const run = vi.fn();
    const messages = new NotificationFeed(present);
    messages.show({ level: "info", message: "Read result", action: { label: "Open", run } });
    const action = present.mock.calls[0]![0].action;
    messages.dispose();
    action.run();
    messages.notify("error", "Late result");
    messages.record({ plugin: "p", level: "warn", message: "Late history" });
    expect(messages.snapshot()).toEqual([]);
    expect(present).toHaveBeenCalledOnce();
    expect(run).not.toHaveBeenCalled();
  });
});
