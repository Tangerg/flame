import { type ReadyHandler } from "@/extensions/types/lifetime";
import { defineExtensionPoint } from "@/extensions/contracts";
export const READY_HANDLER = defineExtensionPoint<ReadyHandler>({
  id: "flame.lifecycle.ready",
  keying: "multi",
});
