// Each shipped language is its own plugin, registering a bundle and contributing a LOCALE
// spec — a third party ships one the same way. The kernel bootstraps English only.

export interface LocaleSpec {
  /**
   * A function rather than the dictionary itself: a statically imported dict puts every
   * language in the entry payload, which makes the per-plugin boundary a fiction exactly
   * where it costs the most. English omits this — it is already loaded.
   */
  load?: () => Promise<Record<string, string>>;
  /** BCP-47 tag, used as both the i18next resource key and the id Settings writes back. */
  id: string;
  /** The language's OWN spelling, so speakers recognise it without needing English. */
  label: string;
  /** Lower comes first. */
  order?: number;
}
