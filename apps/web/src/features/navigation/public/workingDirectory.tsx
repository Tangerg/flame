import { createContext, useContext, type ReactNode } from "react";
import { type WorkingDirectoryAccess } from "@/features/navigation/application/workingDirectoryPicker";
export type { WorkingDirectoryAccess } from "@/features/navigation/application/workingDirectoryPicker";
const WorkingDirectoryContext = createContext<WorkingDirectoryAccess | null>(null);
export function WorkingDirectoryProvider({
  value,
  children,
}: {
  value: WorkingDirectoryAccess;
  children: ReactNode;
}) {
  return <WorkingDirectoryContext value={value}>{children}</WorkingDirectoryContext>;
}
export function useWorkingDirectory(): WorkingDirectoryAccess {
  const value = useContext(WorkingDirectoryContext);
  if (!value) throw new Error("WorkingDirectoryProvider is missing");
  return value;
}
