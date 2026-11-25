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

// Git headers follow LF; embedded CR or Unicode separators remain literal source.
function filePatches(patch: string): string[] {
  const fragments = patch === "" ? [] : patch.split(GIT_DIFF_FILE_BREAK_REGEX);
  const sources: string[] = [];
  for (const fragment of fragments) {
    const previous = sources.at(-1);
    if (previous !== undefined && !previous.endsWith("\n"))
      sources[sources.length - 1] = previous + fragment;
    else sources.push(fragment);
  }
  return sources;
}

function renderPatches(sources: readonly string[]): Map<string, RenderedFile> {
  const rendered = new Map<string, RenderedFile>();
  const readKey = ++nextRead;
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
  return rendered;
}

// The patch covers exactly the manifest's files, each once with its prior path.
function assertManifestMatches(
  manifest: Extract<Diff, { type: "visual" }>["visual"]["manifest"],
  rendered: ReadonlyMap<string, RenderedFile>,
): void {
  if (rendered.size !== manifest.length)
    throw new Error("workspace diff: patch and manifest cover different files");
  const paths = new Set<string>();
  for (const file of manifest) {
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
}

export function parseVisualDiff(diff: Diff) {
  if (diff.type !== "visual")
    throw new Error("workspace diff: expected the visual projection");
  if (diff.visual.patch !== "" && diff.visual.manifest.length === 0)
    throw new Error("workspace diff: nonempty patch has no manifest");
  const rendered = renderPatches(filePatches(diff.visual.patch));
  assertManifestMatches(diff.visual.manifest, rendered);
  return { diff, rendered };
}
