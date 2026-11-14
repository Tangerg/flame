import { parsePatchFiles, type FileDiffMetadata } from "@pierre/diffs";
import type { Diff } from "@flame/runtime-client/wire";

let nextRead = 0;

export function parseVisualDiff(diff: Diff) {
  if (diff.type !== "visual")
    throw new Error("workspace diff: expected the visual projection");
  if (diff.visual.patch !== "" && diff.visual.manifest.length === 0)
    throw new Error("workspace diff: nonempty patch has no manifest");
  const rendered = new Map<string, FileDiffMetadata>();
  for (const patch of parsePatchFiles(diff.visual.patch, {
    cacheKeyPrefix: String(++nextRead),
    throwOnError: true,
    disableLineMatching: true,
  })) {
    for (const file of patch.files) {
      if (rendered.has(file.name))
        throw new Error(`workspace diff: patch repeats ${file.name}`);
      rendered.set(file.name, file);
    }
  }
  if (rendered.size !== diff.visual.manifest.length)
    throw new Error("workspace diff: patch and manifest cover different files");
  const paths = new Set<string>();
  for (const file of diff.visual.manifest) {
    const parsed = rendered.get(file.path);
    if (
      !parsed ||
      paths.has(file.path) ||
      parsed.prevName !== file.previousPath
    )
      throw new Error(
        `workspace diff: patch and manifest disagree for ${file.path}`,
      );
    paths.add(file.path);
  }
  return { diff, rendered };
}
