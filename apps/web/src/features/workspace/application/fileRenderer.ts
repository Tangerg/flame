import { type WorkspaceFileRendererSpec } from "@/features/workspace/public/views";
export function selectFileRenderer(
  renderers: readonly WorkspaceFileRendererSpec[],
  extension: string,
): WorkspaceFileRendererSpec | undefined {
  const matching = renderers.filter((renderer) => renderer.extension === extension);
  return matching.length === 1 ? matching[0] : undefined;
}
