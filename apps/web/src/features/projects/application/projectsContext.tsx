import { createContext, useContext, type ReactNode } from "react";
import { type ProjectsAccess } from "@/features/projects/application/projectsAccess";

const ProjectsContext = createContext<ProjectsAccess | null>(null);

export function ProjectsProvider({
  value,
  children,
}: {
  value: ProjectsAccess;
  children: ReactNode;
}) {
  return <ProjectsContext value={value}>{children}</ProjectsContext>;
}

export function useProjectsAccess(): ProjectsAccess {
  const value = useContext(ProjectsContext);
  if (!value) throw new Error("ProjectsProvider is missing");
  return value;
}
