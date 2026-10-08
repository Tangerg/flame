import { z } from "zod";
import type { PluginCarrier } from "@/foundation/pluginCarrier";

const seedMessage = z.discriminatedUnion("type", [
  z.strictObject({ type: z.literal("flame.carrier.ready.v1") }),
  z.strictObject({ type: z.literal("flame.carrier.failure.v1") }),
]);

export const browserPluginCarrier: PluginCarrier = {
  async open(container, signal, receive) {
    signal.throwIfAborted();
    const frame = document.createElement("iframe");
    frame.title = "Plugin carrier";
    frame.sandbox.add("allow-scripts");
    frame.referrerPolicy = "no-referrer";
    frame.width = "100%";
    frame.height = "100%";
    frame.setAttribute("frameborder", "0");
    const channel = new MessageChannel();
    const initialized = Promise.withResolvers<void>();
    let settled = false;
    let closed = false;
    const close = async () => {
      if (closed) return;
      closed = true;
      clearTimeout(deadline);
      window.removeEventListener("message", ready);
      signal.removeEventListener("abort", abort);
      channel.port1.close();
      channel.port2.close();
      frame.remove();
      if (!settled) {
        settled = true;
        initialized.reject(new Error("Plugin carrier retired."));
      }
    };
    const abort = () => {
      void close();
    };
    const ready = (event: MessageEvent<unknown>) => {
      if (closed || settled || event.source !== frame.contentWindow || event.origin !== "null")
        return;
      const parsed = seedMessage.safeParse(event.data);
      if (!parsed.success) return;
      const value = parsed.data;
      if (value?.type === "flame.carrier.failure.v1") {
        initialized.reject(new Error("Plugin pages are unavailable in this browser."));
        settled = true;
      } else if (value?.type === "flame.carrier.ready.v1") {
        try {
          channel.port1.onmessage = (event) => {
            if (!closed) receive(event.data);
          };
          channel.port1.start();
          frame.contentWindow?.postMessage("flame.carrier.connect.v1", "*", [channel.port2]);
          settled = true;
          initialized.resolve();
          receive({ type: "ready" });
        } catch (error) {
          settled = true;
          initialized.reject(error);
        }
      }
    };
    const deadline = setTimeout(() => {
      initialized.reject(new Error("Plugin carrier did not initialize."));
      settled = true;
    }, 5000);
    window.addEventListener("message", ready);
    signal.addEventListener("abort", abort, { once: true });
    frame.src = "/plugin-carrier.html";
    container.append(frame);
    try {
      await initialized.promise;
      signal.throwIfAborted();
      clearTimeout(deadline);
      window.removeEventListener("message", ready);
      return {
        async send(message) {
          if (closed) throw new Error("Plugin carrier retired.");
          channel.port1.postMessage(message);
        },
        close,
      };
    } catch (error) {
      await close();
      throw error;
    }
  },
};
