import {
  createContext,
  useCallback,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { type HookTrustAccess } from "@/features/hooks/application/hookTrust";

const HookTrustContext = createContext<HookTrustAccess | null>(null);

export function HookTrustProvider({
  value,
  children,
}: {
  value: HookTrustAccess;
  children: ReactNode;
}) {
  return <HookTrustContext value={value}>{children}</HookTrustContext>;
}

export function useSetHookTrust(): (projectRoot: string, trusted: boolean) => Promise<void> {
  const access = useContext(HookTrustContext);
  if (!access) throw new Error("HookTrustProvider is missing");
  const generation = useSyncExternalStore(access.subscribe, access.capture, access.capture);
  return useCallback(
    async (projectRoot, trusted) => {
      if (!generation) throw new GenerationRetiredError("hook_trust_mutation_generation");
      await generation.setProjectTrust(projectRoot, trusted);
    },
    [generation],
  );
}
