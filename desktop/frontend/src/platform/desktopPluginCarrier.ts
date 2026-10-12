import { z } from "zod";
import { pluginMessageFrames } from "./pluginMessageFrames";
import type { DesktopHostBinding } from "./desktopHost";
import type { PluginCarrier } from "@/foundation/pluginCarrier";

const admissionSchema = z.strictObject({
  id: z.string().min(1),
  maxFrameBytes: z.number().int().positive(),
});
const eventSchema = z.strictObject({ id: z.string(), message: z.unknown() });
export function desktopPluginCarrier(binding: DesktopHostBinding): PluginCarrier {
  return {
    async open(container, signal, receive) {
      signal.throwIfAborted();
      let id: string | undefined;
      let closed = false;
      let sending = Promise.resolve();
      const retired = new DOMException("Plugin carrier retired.", "AbortError");
      let maxFrameBytes: number;
      let closing: Promise<void> | undefined;
      let queued: Map<string, unknown[]> | undefined = new Map();
      const unlisten = await binding.on("desktop:plugin-page", (value) => {
        const parsed = eventSchema.safeParse(value);
        if (!parsed.success || closed) return;
        if (queued) {
          const messages = queued.get(parsed.data.id) ?? [];
          messages.push(parsed.data.message);
          queued.set(parsed.data.id, messages);
        } else if (parsed.data.id === id) receive(parsed.data.message);
      });
      const bounds = () => {
        const rect = container.getBoundingClientRect();
        // The native sibling cannot share CSS stacking. Portal lifetime covers
        // the gap between open and exit animation attributes.
        const occluded = document.querySelector("[data-base-ui-portal]") !== null;
        const x = Math.max(0, rect.left),
          y = Math.max(0, rect.top);
        return {
          x,
          y,
          width: occluded ? 0 : Math.max(0, Math.min(rect.right, innerWidth) - x),
          height: occluded ? 0 : Math.max(0, Math.min(rect.bottom, innerHeight) - y),
        };
      };
      const close = () =>
        (closing ??= (async () => {
          closed = true;
          observer.disconnect();
          popups.disconnect();
          removeEventListener("resize", layout);
          removeEventListener("scroll", layout, true);
          signal.removeEventListener("abort", abort);
          unlisten();
          queued?.clear();
          queued = undefined;
          const results = await Promise.allSettled([
            // Publication errors belong to send; retirement still joins that transfer.
            sending.catch(() => undefined),
            id ? binding.call("main.DesktopHost.ClosePluginPage", id) : undefined,
          ]);
          const errors = results.flatMap((result) =>
            result.status === "rejected" ? [result.reason] : [],
          );
          if (errors.length) throw new AggregateError(errors, "Plugin carrier retirement failed.");
        })());
      const layout = () => {
        if (id && !closed)
          void binding.call("main.DesktopHost.PositionPluginPage", id, bounds()).catch(() => {
            if (!closed)
              receive({
                type: "failure",
                reason: "The native plugin surface could not be positioned.",
              });
          });
      };
      const observer = new ResizeObserver(layout);
      const popups = new MutationObserver(layout);
      const abort = () => {
        void close().catch(console.error);
      };
      try {
        signal.throwIfAborted();
        const admission = admissionSchema.parse(
          await binding.call("main.DesktopHost.OpenPluginPage", bounds()),
        );
        id = admission.id;
        maxFrameBytes = admission.maxFrameBytes;
        if (signal.aborted) {
          await close();
          signal.throwIfAborted();
        }
        observer.observe(container);
        popups.observe(document.body, {
          subtree: true,
          childList: true,
        });
        addEventListener("resize", layout);
        addEventListener("scroll", layout, true);
        signal.addEventListener("abort", abort, { once: true });
        await binding.call("main.DesktopHost.PositionPluginPage", id, bounds());
        signal.throwIfAborted();
        const pending = queued?.get(id) ?? [];
        queued = undefined;
        for (const message of pending) {
          if (closed) break;
          receive(message);
        }
        return {
          send(message) {
            if (closed) return Promise.reject(retired);
            const frames = pluginMessageFrames(message, maxFrameBytes);
            sending = sending.then(async () => {
              for (const frame of frames) {
                if (closed) throw retired;
                await binding.call("main.DesktopHost.SendPluginPage", id, frame);
              }
            });
            return sending;
          },
          close,
        };
      } catch (error) {
        await close();
        throw error;
      }
    },
  };
}
