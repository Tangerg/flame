import type { ToolPreviewComponent } from "@/plugins/sdk";

export interface ToolPreviewContribution {
  key: string;
  component: ToolPreviewComponent;
}

export function toolPreviews(
  components: Readonly<Record<string, ToolPreviewComponent>>,
): ToolPreviewContribution[] {
  return Object.entries(components).map(([key, component]) => ({ key, component }));
}
