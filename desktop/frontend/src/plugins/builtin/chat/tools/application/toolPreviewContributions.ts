import type { ToolPreviewComponent } from "@/plugins/sdk";

export interface ToolPreviewContribution {
  key: string;
  component: ToolPreviewComponent;
}

/** Keyed by RUNTIME tool name, so the map at a callsite is the registration itself. */
export function toolPreviews(
  components: Readonly<Record<string, ToolPreviewComponent>>,
): ToolPreviewContribution[] {
  return Object.entries(components).map(([key, component]) => ({ key, component }));
}
