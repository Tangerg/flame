import { type ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { type AnyPlugin, type Host } from "dougong";
interface Props {
  children: ReactNode;
  plugins: AnyPlugin[];
}

export function PluginProvider({ children, plugins }: Props) {
  // Successive kernels share renderer-owned features and must not overlap their lifetimes.
  const predecessor = useRef<Promise<void>>(Promise.resolve());
  const installation = useMemo(() => ({ plugins }), [plugins]);
  const [outcome, setOutcome] = useState<
    | { installation: object; status: "ready" }
    | { installation: object; status: "failed"; error: Error }
    | null
  >(null);

  useEffect(() => {
    const controller = new AbortController();
    const retirement = new Promise<void>((resolve) => {
      controller.signal.addEventListener("abort", () => resolve(), { once: true });
    });
    const previous = predecessor.current;
    const lifetime = (async () => {
      await previous;
      if (controller.signal.aborted) return;
      let host: Host;
      try {
        host = await startKernel(installation.plugins, controller.signal);
      } catch (error) {
        if (controller.signal.aborted && error === controller.signal.reason) return;
        throw error;
      }
      if (!controller.signal.aborted) setOutcome({ installation, status: "ready" });
      await retirement;
      await stopKernel(host);
    })();
    predecessor.current = lifetime;
    void lifetime.catch((error: unknown) => {
      if (controller.signal.aborted) console.error("[plugin] kernel retirement failed:", error);
      else
        setOutcome({
          installation,
          status: "failed",
          error: new Error("plugin kernel lifecycle failed", { cause: error }),
        });
    });

    return () => {
      controller.abort();
    };
  }, [installation]);

  if (outcome?.installation !== installation) return null;
  if (outcome.status === "failed") throw outcome.error;

  return children;
}
