import { z } from "zod";
import type { DesktopHostBinding } from "./desktopHost";
import type { PluginCarrier } from "@/foundation/pluginCarrier";

const eventSchema = z.strictObject({ id: z.string(), message: z.unknown() });
export function desktopPluginCarrier(binding: DesktopHostBinding): PluginCarrier {
  return {
    async open(container, signal, receive) {
      signal.throwIfAborted();
      let id: string | undefined;
      let closed = false;
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
        // A sibling native view cannot participate in the workbench's CSS stacking.
        // Its visibility follows the rendered popup owner, including exit motion.
        const occluded =
          document.querySelector(
            "[data-base-ui-portal] [data-open], [data-base-ui-portal] [data-ending-style]",
          ) !== null;
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
          if (id) await binding.call("main.DesktopHost.ClosePluginPage", id);
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
        id = z
          .string()
          .min(1)
          .parse(await binding.call("main.DesktopHost.OpenPluginPage", bounds()));
        if (signal.aborted) {
          await close();
          signal.throwIfAborted();
        }
        observer.observe(container);
        popups.observe(document.body, {
          subtree: true,
          childList: true,
          attributes: true,
          attributeFilter: ["data-open", "data-ending-style"],
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
          async send(message) {
            if (closed) throw new Error("Plugin carrier retired.");
            await binding.call("main.DesktopHost.SendPluginPage", id, JSON.stringify(message));
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
