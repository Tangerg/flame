import { beforeEach, describe, expect, it } from "vitest";
import { useNotificationStore } from "./notifications";

beforeEach(() => useNotificationStore.setState({ log: [] }));

describe("notification identity", () => {
  it("issues exact decimal identities and dismisses only the selected entry", () => {
    const store = useNotificationStore.getState();
    const first = store.push({ plugin: "p", level: "info", message: "first" });
    const second = store.push({ plugin: "p", level: "info", message: "second" });

    expect(first.id).toMatch(/^\d+$/);
    expect(BigInt(second.id)).toBe(BigInt(first.id) + 1n);
    useNotificationStore.getState().dismiss(first.id);

    expect(useNotificationStore.getState().log).toEqual([{ ...first, dismissed: true }, second]);
  });
});
