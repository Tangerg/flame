import { asyncDisposeSymbol } from "dougong";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { type LocaleSpec } from "@/features/i18n/public/types";
import { activeLocale, mountLocaleBundle } from "@/lib/i18n/index";

export interface LocaleSource {
  readonly key: string;
  readonly tag: string;
  readonly title: string;
  load(signal: AbortSignal): Promise<Record<string, string>>;
}

export function contributeLocale(
  scope: Pick<ContributionLifetime, "signal" | "spawn" | "cleanup" | "contribute">,
  spec: Omit<LocaleSpec, "activate"> & {
    load(signal: AbortSignal): Promise<Record<string, string>>;
  },
): void {
  let activation: Promise<void> | undefined;
  const activate = (): Promise<void> => {
    scope.signal.throwIfAborted();
    if (!activation) {
      const attempt = scope.spawn(async (signal) => {
        const dictionary = await spec.load(signal);
        signal.throwIfAborted();
        scope.cleanup(mountLocaleBundle(spec.id, dictionary));
      }).result;
      activation = attempt;
      void attempt.catch(() => {
        if (activation === attempt) activation = undefined;
      });
    }
    return activation;
  };
  scope.contribute(LOCALE, { id: spec.id, label: spec.label, order: spec.order, activate });
  if (spec.id === activeLocale()) void activate();
}

export function createLocaleReconciler(
  scope: ContributionLifetime,
  report: (error: Error) => void,
): (sources: readonly LocaleSource[]) => Promise<void> {
  const current = new Map<string, { signature: string; lifetime: ContributionLifetime }>();
  return async (sources) => {
    const byTag = new Map<string, LocaleSource[]>();
    for (const source of sources) {
      const group = byTag.get(source.tag) ?? [];
      group.push(source);
      byTag.set(source.tag, group);
    }
    for (const [tag, resource] of current) {
      const signature = JSON.stringify((byTag.get(tag) ?? []).map((s) => s.key).sort());
      if (signature === resource.signature) continue;
      current.delete(tag);
      await resource.lifetime[asyncDisposeSymbol]();
    }
    for (const [tag, group] of byTag) {
      if (scope.signal.aborted) return;
      if (current.has(tag)) continue;
      const lifetime = scope.lifetime(`locale:${tag}`);
      current.set(tag, {
        signature: JSON.stringify(group.map((source) => source.key).sort()),
        lifetime,
      });
      const source = group[0];
      if (tag === "en" || group.length !== 1 || !source) {
        report(
          new Error(
            `locale "${tag}" has conflicting sources: ${group.map((s) => s.key).join(", ")}`,
          ),
        );
        continue;
      }
      contributeLocale(lifetime, {
        id: tag,
        label: source.title,
        load: (signal) => source.load(signal),
      });
    }
  };
}
