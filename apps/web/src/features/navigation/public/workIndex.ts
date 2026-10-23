import type { ComponentType } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
export interface WorkIndexItemSpec {
  id: string;
  order?: number;
  component: ComponentType;
}
export const WORK_INDEX_ITEM = defineExtensionPoint<WorkIndexItemSpec>({
  id: "flame.workIndex.item",
  keying: "single",
});
export function useWorkIndexItems(): WorkIndexItemSpec[] {
  return useExtensionPoint(WORK_INDEX_ITEM);
}
import { type Contributor } from "@/extensions/definePlugin";
import { type Disposable } from "@/extensions/types/lifetime";
export {
  createNewSession,
  useWorkIndexActions,
  type WorkIndexActions,
} from "@/features/navigation/application/workIndexActions";
export { useWorkIndex } from "@/features/navigation/application/useWorkIndex";
export type { WorkGroup, WorkProject, WorkSession } from "@/features/navigation/domain/workIndex";
export {
  sessionIsRunning,
  sessionIsWaiting,
  sessionNeedsAttention,
  workGroupHasRunning,
  workGroupWaitingCount,
} from "@/features/navigation/domain/workIndex";

export function contributeWorkIndexItem(ctx: Contributor, item: WorkIndexItemSpec): Disposable {
  return ctx.contribute(WORK_INDEX_ITEM, item);
}
