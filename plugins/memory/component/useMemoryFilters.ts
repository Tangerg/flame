import { useRef, useState } from "react";

// "all" lifts a filter; every other value keeps only matching items.
export const UNFILTERED = "all";
export const STATUS_FILTERS = [UNFILTERED, "pending", "active"] as const;
export const ORIGIN_FILTERS = [UNFILTERED, "auto", "user"] as const;
export type StatusFilter = (typeof STATUS_FILTERS)[number];
export type OriginFilter = (typeof ORIGIN_FILTERS)[number];

export function keeps(filter: string, value: string): boolean {
  return filter === UNFILTERED || filter === value;
}

// Filters narrow the accepted page on the client; they never reach the backend.
export function useMemoryFilters() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>(UNFILTERED);
  const [origin, setOrigin] = useState<OriginFilter>(UNFILTERED);
  const [pinned, setPinned] = useState(false);
  const [open, setOpen] = useState(false);
  const searchElement = useRef<HTMLInputElement>(null);
  function clear() {
    setStatus(UNFILTERED);
    setOrigin(UNFILTERED);
    setPinned(false);
  }
  return {
    search,
    setSearch,
    status,
    setStatus,
    origin,
    setOrigin,
    pinned,
    setPinned,
    open,
    setOpen,
    searchElement,
    active:
      Number(status !== UNFILTERED) +
      Number(origin !== UNFILTERED) +
      Number(pinned),
    clear,
    reset() {
      setSearch("");
      clear();
    },
  };
}

export type MemoryFilterState = ReturnType<typeof useMemoryFilters>;
