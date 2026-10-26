import { createContext, useContext, type ReactNode } from "react";
import { type WorkspaceAccess } from "@/features/workspace/application/workspaceAccess";
export type { WorkspaceAccess } from "@/features/workspace/application/workspaceAccess";
const WorkspaceContext = createContext<WorkspaceAccess | null>(null);
export function WorkspaceProvider({
  value,
  children,
}: {
  value: WorkspaceAccess;
  children: ReactNode;
}) {
  return <WorkspaceContext value={value}>{children}</WorkspaceContext>;
}
export function useWorkspace(): WorkspaceAccess {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("WorkspaceProvider is missing");
  return value;
}
