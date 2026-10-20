import { type Contributor } from "@/extensions/definePlugin";
import { LAYOUT_SLOT } from "@/extensions/layout";
import { type Disposable } from "@/extensions/types/lifetime";
import { type LayoutSlotSpec } from "@/extensions/layout";
export function contributeLayout(ctx: Contributor, slot: string, spec: LayoutSlotSpec): Disposable {
  return ctx.contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` });
}
