import { type WorkspaceFileRendererSpec } from "@/features/workspace/public/views";
export function fileRendererCandidates(
  renderers: readonly WorkspaceFileRendererSpec[],
  extension: string,
): WorkspaceFileRendererSpec[] {
  return renderers.filter((renderer) => renderer.extension === extension);
}
