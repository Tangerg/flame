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
  const [readyInstallation, setReadyInstallation] = useState<object | null>(null);

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
        setReadyInstallation(installation);
      } catch (error) {
        if (!retired) console.error("[plugin] kernel startup failed:", error);
      }
    })();

    return () => {
      retired = true;
      controller.abort();
      void disposeOwnedResources();
    };
  }, [installation]);

  if (readyInstallation !== installation) return null;

  return <TooltipProvider>{children}</TooltipProvider>;
}
