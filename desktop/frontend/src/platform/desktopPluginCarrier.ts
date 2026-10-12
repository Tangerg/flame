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
      let animationFrame: number | undefined;
      let positioned: string | undefined;
      const bounds = () => {
        const frame = container.getBoundingClientRect();
        let left = Math.max(0, frame.left);
        let top = Math.max(0, frame.top);
        let right = Math.min(innerWidth, frame.right);
        let bottom = Math.min(innerHeight, frame.bottom);
        let hidden = document.querySelector("[data-base-ui-portal]") !== null;
        for (
          let element: HTMLElement | null = container;
          element;
          element = element.parentElement
        ) {
          const style = getComputedStyle(element);
          hidden ||=
            Boolean(element.hidden) ||
            style.display === "none" ||
            style.visibility === "hidden" ||
            style.visibility === "collapse" ||
            style.opacity === "0";
          if (element === container) continue;
          const rect = element.getBoundingClientRect();
          const scaleX = element.offsetWidth ? rect.width / element.offsetWidth : 1;
          const scaleY = element.offsetHeight ? rect.height / element.offsetHeight : 1;
          if (["hidden", "clip", "auto", "scroll"].includes(style.overflowX)) {
            const x = rect.left + element.clientLeft * scaleX;
            left = Math.max(left, x);
            right = Math.min(right, x + element.clientWidth * scaleX);
          }
          if (["hidden", "clip", "auto", "scroll"].includes(style.overflowY)) {
            const y = rect.top + element.clientTop * scaleY;
            top = Math.max(top, y);
            bottom = Math.min(bottom, y + element.clientHeight * scaleY);
          }
        }
        return {
          frame: { x: frame.x, y: frame.y, width: frame.width, height: frame.height },
          clip: {
            x: left,
            y: top,
            width: hidden ? 0 : Math.max(0, right - left),
            height: hidden ? 0 : Math.max(0, bottom - top),
          },
        };
      };
      const close = () =>
        (closing ??= (async () => {
          closed = true;
          if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
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
      // DOM owns geometry, including position changes that ResizeObserver cannot observe.
      // Await each native projection so a stale layout cannot overtake its successor.
      const layout = async () => {
        if (!id || closed) return;
        if (!container.isConnected) {
          await close();
          return;
        }
        const next = bounds();
        const signature = JSON.stringify(next);
        if (signature !== positioned) {
          await binding.call("main.DesktopHost.PositionPluginPage", id, next);
          positioned = signature;
        }
        if (!closed)
          animationFrame = requestAnimationFrame(() => {
            void layout().catch(() => {
              if (!closed)
                receive({
                  type: "failure",
                  reason: "The native plugin surface could not be positioned.",
                });
            });
          });
      };
      const abort = () => {
        void close().catch(console.error);
      };
      try {
        signal.throwIfAborted();
        if (!container.isConnected) throw retired;
        const admission = admissionSchema.parse(
          await binding.call("main.DesktopHost.OpenPluginPage", bounds()),
        );
        id = admission.id;
        maxFrameBytes = admission.maxFrameBytes;
        if (signal.aborted) {
          await close();
          signal.throwIfAborted();
        }
        signal.addEventListener("abort", abort, { once: true });
        await layout();
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
