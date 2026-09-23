export interface LocaleSpec {
  load?: () => Promise<Record<string, string>>;
  id: string;
  label: string;
  order?: number;
}
