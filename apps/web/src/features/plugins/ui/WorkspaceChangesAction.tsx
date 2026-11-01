import { useLayoutEffect, useMemo, useRef } from "react";
import type { WorkspaceChangesActionInput } from "@flame/component-sdk";
import { useChangedFiles } from "@/features/workspace/public/queries";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useActiveSessionWorkspace, resolvedWorkspaceCwd } from "@/features/agent/public/session";
import { openWorkspaceViewAtPath } from "@/features/workspace/public/navigation";
import { InstalledComponent } from "./InstalledComponent";
import type { ComponentProps } from "react";

export function WorkspaceChangesAction({
  targetView,
  ...props
}: ComponentProps<typeof InstalledComponent> & { targetView: string }) {
  const workspace = useWorkspace();
  const cwd = resolvedWorkspaceCwd(useActiveSessionWorkspace());
  const changes = useChangedFiles();
  const target = useMemo(() => ({ cwd, targetView }), [cwd, targetView]);
  const active = useRef<typeof target | null>(null);
  useLayoutEffect(() => {
    active.current = target;
    return () => {
      active.current = null;
    };
  }, [target]);
  const input = useMemo<WorkspaceChangesActionInput>(
    () => ({
      changes: Object.freeze((changes ?? []).map((change) => Object.freeze({ ...change }))),
      openView(origin) {
        props.resources.throwIfAborted();
        props.lifetime.signal.throwIfAborted();
        if (active.current !== target) return;
        openWorkspaceViewAtPath(workspace, targetView, "", origin);
      },
    }),
    [changes, target, props.lifetime, props.resources, targetView, workspace],
  );
  if (!cwd || !changes?.length) return null;
  return <InstalledComponent {...props} input={input} />;
}
