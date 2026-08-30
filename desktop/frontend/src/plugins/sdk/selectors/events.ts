// The reducer calls these per dispatch, so both are O(1) through the cached per-point index.

import type { StreamEventHandler } from "../types";
import { STREAM_EVENT_HANDLER } from "../kernelPoints";
import { contributionsTo } from "../kernel";
import { createPointSubIndex } from "./extensions";

type StreamHandlerItem = { eventType: string; handler: StreamEventHandler };

const coreByType = createPointSubIndex((item: StreamHandlerItem, pluginName) => ({
  key: item.eventType,
  value: { pluginName, handler: item.handler },
}));

/** Insertion order; the reducer chains them through the state. */
export function lookupStreamHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: StreamEventHandler }> {
  return coreByType(contributionsTo(STREAM_EVENT_HANDLER)).get(eventType) ?? [];
}
