import type { WorkIndexItemSpec } from "@/plugins/sdk/types/navigation";
import type { Disposable } from "@/plugins/sdk/types/common";
import { useWorkIndexItems as useSdkWorkIndexItems } from "@/plugins/sdk/selectors/layout";
import { WORK_INDEX_ITEM } from "@/plugins/sdk/kernelPoints";
import type { Contributor } from "@/plugins/sdk";

export type { WorkIndexItemSpec } from "@/plugins/sdk/types/navigation";

export function contributeWorkIndexItem(ctx: Contributor, item: WorkIndexItemSpec): Disposable {
  return ctx.contribute(WORK_INDEX_ITEM, item);
}

export function useWorkIndexItems(): WorkIndexItemSpec[] {
  return useSdkWorkIndexItems();
}
