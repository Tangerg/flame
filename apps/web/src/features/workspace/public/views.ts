import { useMemo } from "react";
import { useLocale } from "@/lib/i18n/index";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import { type ComponentType, type ReactNode } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
export interface WorkspaceFileRendererProps {
  cwd?: string;
  path: string;
  content: string;
  startLine: number;
  totalLines: number;
  truncated?: boolean;
  fallback: ReactNode;
}

export interface WorkspaceFileRendererSpec {
  id: string;
  title: string;
  extension: string;
  component: ComponentType<WorkspaceFileRendererProps>;
}

export type ContextDockDestinationScope = "workspace" | "session" | "run";

export interface WorkspaceViewSpec {
  material?: string;
  id: string;
  title: string;
  localizedTitles?: Readonly<Record<string, string>>;
  icon?: string;
  badge?: ComponentType;
  order?: number;
  dock?: ContextDockDestinationScope;
  component: ComponentType;
}

export const WORKSPACE_FILE_RENDERER = defineExtensionPoint<WorkspaceFileRendererSpec>({
  id: "flame.workspace.fileRenderer",
  keying: "single",
});

export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({
  id: "flame.workspaceView",
  keying: "single",
});

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  const entries = useExtensionPoint(WORKSPACE_VIEW);
  const locale = useLocale();
  return useMemo(
    () =>
      entries.map((entry) =>
        entry.localizedTitles
          ? { ...entry, title: presentationTitle(entry.title, entry.localizedTitles, locale) }
          : entry,
      ),
    [entries, locale],
  );
}
