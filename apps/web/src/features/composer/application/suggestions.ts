export interface SuggestionList<T> {
  key: string;
  open: boolean;
  items: readonly T[];
  accept: (item: T) => void;
  dismiss: () => void;
}
