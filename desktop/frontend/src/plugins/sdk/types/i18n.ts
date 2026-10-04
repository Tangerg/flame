export interface LocaleSpec {
  activate(): Promise<void>;
  id: string;
  label: string;
  order?: number;
}
