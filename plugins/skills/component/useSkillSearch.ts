import { useRef, useState, type RefObject } from "react";
import { type SkillRecord } from "./types";

// The catalog query filters the accepted rows on the client; a new query
// starts the reader at the top.
export function useSkillSearch(viewport: RefObject<HTMLDivElement | null>) {
  const [query, setQuery] = useState("");
  const field = useRef<HTMLInputElement>(null);
  return {
    query,
    field,
    change(next: string) {
      if (next === query) return;
      setQuery(next);
      if (viewport.current) viewport.current.scrollTop = 0;
    },
    reset() {
      setQuery("");
    },
    matching(rows: SkillRecord[]): SkillRecord[] {
      const needle = query.toLocaleLowerCase();
      return rows.filter((item) =>
        `${item.name} ${item.description ?? ""} ${"scope" in item ? item.scope : "lifecycle" in item ? item.lifecycle : ""}`
          .toLocaleLowerCase()
          .includes(needle),
      );
    },
  };
}
