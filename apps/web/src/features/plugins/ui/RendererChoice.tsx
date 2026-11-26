import { Select } from "@flame/ui";
import { useCallback, useSyncExternalStore } from "react";
import { useInstalledPlugins } from "@/features/plugins/context";
import { useT, labelText, type Label } from "@/lib/i18n/index";
import { tupleKey } from "@/lib/tupleKey";

interface Renderer {
  id: string;
  title: Label;
}

type RendererKind = "file" | "tool";

function preferenceKey(endpoint: string, kind: RendererKind, target: string): string {
  return tupleKey(endpoint, kind, target);
}

export function resolveRenderer<T extends Renderer>(
  candidates: readonly T[],
  preferred: string | null,
): T | undefined {
  if (preferred !== null) return candidates.find(({ id }) => id === preferred);
  return candidates.length === 1 ? candidates[0] : undefined;
}

export function useRendererChoice<T extends Renderer>(
  kind: RendererKind,
  target: string,
  candidates: readonly T[],
) {
  const { presentationPreferences } = useInstalledPlugins();
  const key = preferenceKey(presentationPreferences.endpoint(), kind, target);
  const preferred = useSyncExternalStore(presentationPreferences.subscribe, () =>
    presentationPreferences.read(key),
  );
  return {
    renderer: resolveRenderer(candidates, preferred),
    control:
      candidates.length > 1 ||
      (preferred !== null && !candidates.some(({ id }) => id === preferred)) ? (
        <RendererPicker
          candidates={candidates}
          preferred={preferred}
          choose={(id) => presentationPreferences.choose(key, id)}
        />
      ) : undefined,
  };
}

export function useRendererResolver(kind: RendererKind) {
  const { presentationPreferences } = useInstalledPlugins();
  const endpoint = presentationPreferences.endpoint();
  const selections = useSyncExternalStore(
    presentationPreferences.subscribe,
    presentationPreferences.selections,
  );
  return useCallback(
    <T extends Renderer>(target: string, candidates: readonly T[]) =>
      resolveRenderer(candidates, selections[preferenceKey(endpoint, kind, target)] ?? null),
    [endpoint, kind, selections],
  );
}

function RendererPicker({
  candidates,
  preferred,
  choose,
}: {
  candidates: readonly Renderer[];
  preferred: string | null;
  choose: (id: string | null) => void;
}) {
  const t = useT();
  const automatic = "";
  const options = [
    { value: automatic, label: t("renderer.automatic") },
    ...candidates.map(({ id, title }) => {
      const text = labelText(title, t);
      const ambiguous = candidates.some(
        (other) => other.id !== id && labelText(other.title, t) === text,
      );
      return { value: id, label: ambiguous ? text + " · " + id : text };
    }),
  ];
  if (preferred && !candidates.some(({ id }) => id === preferred))
    options.push({ value: preferred, label: t("renderer.unavailable") });
  return (
    <Select
      label={t("renderer.choose")}
      value={preferred ?? automatic}
      options={options}
      onValueChange={(value) => choose(value || null)}
    />
  );
}
