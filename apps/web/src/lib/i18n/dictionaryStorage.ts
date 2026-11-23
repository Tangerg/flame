import { z } from "zod";
import { type Locale } from "./locale";

const DICTIONARY_STORAGE_KEY = "flame.locale-dictionary";

const storedDictionarySchema = z.object({
  locale: z.string().min(1),
  source: z.string().min(1),
  messages: z.record(z.string(), z.string()),
});

export type StoredDictionary = z.infer<typeof storedDictionarySchema>;

export function readStoredDictionary(locale: Locale): StoredDictionary | undefined {
  try {
    const raw = localStorage.getItem(DICTIONARY_STORAGE_KEY);
    if (raw === null) return undefined;
    const parsed = storedDictionarySchema.safeParse(JSON.parse(raw));
    return parsed.success && parsed.data.locale === locale ? parsed.data : undefined;
  } catch {
    return undefined;
  }
}

export function storeDictionary(dictionary: StoredDictionary): void {
  try {
    localStorage.setItem(DICTIONARY_STORAGE_KEY, JSON.stringify(dictionary));
  } catch {}
}

export function forgetStoredDictionary(): void {
  try {
    localStorage.removeItem(DICTIONARY_STORAGE_KEY);
  } catch {}
}
