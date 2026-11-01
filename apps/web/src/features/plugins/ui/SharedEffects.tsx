import { useMemo, useSyncExternalStore } from "react";
import type { ComponentType } from "react";
import { Select } from "@flame/ui";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { useInstalledPlugins } from "@/features/plugins/context";
import { tupleKey } from "@/lib/tupleKey";
import { useT } from "@/lib/i18n/index";

interface SharedEffect {
  group: string;
  id: string;
  title: string;
  component: ComponentType;
}

export const SHARED_CLIENT_EFFECT = defineExtensionPoint<SharedEffect>({
  id: "flame.installed.sharedClientEffect",
  keying: "single",
});

function useSharedEffects() {
  const effects = useExtensionPoint(SHARED_CLIENT_EFFECT);
  return useMemo(
    () =>
      Map.groupBy(
        effects.toSorted((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)),
        (e) => e.group,
      ),
    [effects],
  );
}

function useSelectedEffect(group: string, candidates: readonly SharedEffect[]) {
  const { presentationPreferences } = useInstalledPlugins();
  const key = tupleKey(presentationPreferences.endpoint(), "effect", group);
  const preferred = useSyncExternalStore(presentationPreferences.subscribe, () =>
    presentationPreferences.read(key),
  );
  return {
    selected: preferred === null ? candidates[0] : candidates.find((e) => e.id === preferred),
    preferred,
    choose: (value: string | null) => presentationPreferences.choose(key, value),
  };
}

function ActiveEffect({ group, candidates }: { group: string; candidates: SharedEffect[] }) {
  const { selected } = useSelectedEffect(group, candidates);
  return selected ? <selected.component key={selected.id} /> : null;
}

export function SharedEffects() {
  return [...useSharedEffects()].map(([group, candidates]) => (
    <ActiveEffect key={group} group={group} candidates={candidates} />
  ));
}

function EffectSetting({ group, candidates }: { group: string; candidates: SharedEffect[] }) {
  const { preferred, selected, choose } = useSelectedEffect(group, candidates);
  const t = useT();
  if (candidates.length === 1 && selected) return null;
  const options = [
    { value: "", label: t("renderer.automatic") },
    ...candidates.map((e) => ({ value: e.id, label: e.title + " · " + e.id })),
  ];
  if (preferred && !selected) options.push({ value: preferred, label: t("renderer.unavailable") });
  return (
    <Select
      label={t("packages.effects.choose", { name: candidates[0]!.title })}
      options={options}
      value={preferred ?? ""}
      onValueChange={(id) => choose(id || null)}
    />
  );
}
export function SharedEffectSettings() {
  return [...useSharedEffects()].map(([group, candidates]) => (
    <EffectSetting key={group} group={group} candidates={candidates} />
  ));
}
