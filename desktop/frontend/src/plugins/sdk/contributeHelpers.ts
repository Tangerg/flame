// Layout contributions need a STABLE id, or re-registering a slot entry stacks rather than
// replaces. Free functions over an explicit ctx, so a plugin has to import what it uses.

import type { Contributor } from "./definePlugin";
import { LAYOUT_SLOT } from "./kernelPoints";
import type { Disposable } from "./types/common";
import type { LayoutSlotSpec } from "./types/workspace";

export function contributeLayout(ctx: Contributor, slot: string, spec: LayoutSlotSpec): Disposable {
  return ctx.contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` });
}
