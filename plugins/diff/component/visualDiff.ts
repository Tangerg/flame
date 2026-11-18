import {
  GIT_DIFF_FILE_BREAK_REGEX,
  processFile,
  type FileDiffMetadata,
} from "@pierre/diffs";
import type { Diff } from "@flame/runtime-client/wire";

type RenderedFile = {
  readonly source: string;
  readonly file: FileDiffMetadata;
};

let nextRead = 0;

export function parseVisualDiff(diff: Diff) {
  if (diff.type !== "visual")
    throw new Error("workspace diff: expected the visual projection");
  if (diff.visual.patch !== "" && diff.visual.manifest.length === 0)
    throw new Error("workspace diff: nonempty patch has no manifest");
  const rendered = new Map<string, RenderedFile>();
  const readKey = ++nextRead;
  const fragments =
    diff.visual.patch === ""
      ? []
      : diff.visual.patch.split(GIT_DIFF_FILE_BREAK_REGEX);
  const sources: string[] = [];
  // Git headers follow LF; embedded CR or Unicode separators remain literal source.
  for (const fragment of fragments) {
    const previous = sources.at(-1);
    if (previous !== undefined && !previous.endsWith("\n"))
      sources[sources.length - 1] = previous + fragment;
    else sources.push(fragment);
  }
  for (const [index, source] of sources.entries()) {
    if (!source.startsWith("diff --git "))
      throw new Error("workspace diff: expected a Git file patch");
    const file = processFile(source, {
      cacheKey: `workspace-diff:${readKey}:${index}`,
      isGitDiff: true,
      throwOnError: true,
      disableLineMatching: true,
    });
    if (!file)
      throw new Error("workspace diff: file patch produced no metadata");
    if (rendered.has(file.name))
      throw new Error(`workspace diff: patch repeats ${file.name}`);
    rendered.set(file.name, { source, file });
  }
  if (rendered.size !== diff.visual.manifest.length)
    throw new Error("workspace diff: patch and manifest cover different files");
  const paths = new Set<string>();
  for (const file of diff.visual.manifest) {
    const parsed = rendered.get(file.path);
    if (
      !parsed ||
      paths.has(file.path) ||
      parsed.file.prevName !== file.previousPath
    )
      throw new Error(
        `workspace diff: patch and manifest disagree for ${file.path}`,
      );
    paths.add(file.path);
  }
  return { diff, rendered };
}
