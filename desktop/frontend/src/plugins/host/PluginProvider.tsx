import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { TooltipProvider } from "@/ui";
import { startKernel, stopKernel } from "../sdk";
import type { AnyPlugin, Host } from "dougong";

interface Props {
  children: ReactNode;
  plugins: AnyPlugin[];
}

export function PluginProvider({ children, plugins }: Props) {
  const installation = useMemo(() => ({ plugins }), [plugins]);
  const [outcome, setOutcome] = useState<
    | { installation: object; status: "ready" }
    | { installation: object; status: "failed"; error: Error }
    | null
  >(null);

  useEffect(() => {
    const controller = new AbortController();
    let retired = false;
    let host: Host | undefined;
    let disposal: Promise<void> | undefined;

    const disposeOwnedResources = () => {
      if (!host || disposal) return disposal;
      const ownedHost = host;
      disposal = stopKernel(ownedHost).catch((error: unknown) => {
        console.error("[plugin] kernel teardown failed:", error);
      });
      return disposal;
    };

    void (async () => {
      try {
        host = await startKernel(installation.plugins, controller.signal);
        if (retired) {
          void disposeOwnedResources();
          return;
        }
        setOutcome({ installation, status: "ready" });
      } catch (error) {
        if (!retired) {
          setOutcome({
            installation,
            status: "failed",
            error: new Error("plugin kernel startup failed", { cause: error }),
          });
        }
      }
    })();

    return () => {
      retired = true;
      controller.abort();
      void disposeOwnedResources();
    };
  }, [installation]);

  if (outcome?.installation !== installation) return null;
  if (outcome.status === "failed") throw outcome.error;

  return <TooltipProvider>{children}</TooltipProvider>;
}
