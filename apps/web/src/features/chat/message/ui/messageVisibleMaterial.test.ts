import { describe, expect, it, vi } from "vitest";
import { MessageVisibleMaterialOwner } from "@/features/chat/message/ui/messageVisibleMaterial";

describe("MessageVisibleMaterialOwner", () => {
  it("does not let predecessor teardown retire a successor projection", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const predecessor = Symbol("predecessor");
    const successor = Symbol("successor");
    const generation = {};

    owner.observe(predecessor, generation, false);
    owner.observe(successor, generation, false);
    owner.retire(predecessor);

    expect(owner.actionsMaterialization("settled", generation)).toBe("active");

    owner.observe(successor, generation, true);
    expect(owner.actionsMaterialization("settled", generation)).toBe("settled");
  });

  it("does not lend predecessor settlement to a successor accepted generation", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const projection = Symbol("projection");
    const predecessor = {};
    const successor = {};

    owner.observe(projection, predecessor, true);

    expect(owner.actionsMaterialization("settled", successor)).toBe("active");

    owner.observe(projection, successor, true);
    expect(owner.actionsMaterialization("settled", successor)).toBe("settled");
  });

  it("does not publish when an active projection advances between active generations", () => {
    const owner = new MessageVisibleMaterialOwner("session-owner", "message-owner");
    const projection = Symbol("projection");
    const changed = vi.fn();
    owner.subscribe(changed);

    owner.observe(projection, {}, false);
    changed.mockClear();
    owner.observe(projection, {}, false);

    expect(changed).not.toHaveBeenCalled();
  });
});
