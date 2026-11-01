import { Select } from "@flame/ui";
import { useSyncExternalStore } from "react";
import { useInstalledPlugins } from "@/features/plugins/context";
import { useT } from "@/lib/i18n/index";
import { tupleKey } from "@/lib/tupleKey";

interface Renderer {
  id: string;
  title: string;
}

export function resolveRenderer<T extends Renderer>(
  candidates: readonly T[],
  preferred: string | null,
): T | undefined {
  if (preferred !== null) return candidates.find(({ id }) => id === preferred);
  return candidates.length === 1 ? candidates[0] : undefined;
}

export function useRendererChoice<T extends Renderer>(
  kind: "file" | "tool",
  target: string,
  candidates: readonly T[],
) {
  const { presentationPreferences } = useInstalledPlugins();
  const key = tupleKey(presentationPreferences.endpoint(), kind, target);
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
    ...candidates.map(({ id, title }) => ({
      value: id,
      label: candidates.some((other) => other.id !== id && other.title === title)
        ? title + " · " + id
        : title,
    })),
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
