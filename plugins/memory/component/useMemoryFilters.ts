import { useRef, useState } from "react";

// Filters narrow the accepted page on the client; they never reach the backend.
export function useMemoryFilters() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
  const [open, setOpen] = useState(false);
  const searchElement = useRef<HTMLInputElement>(null);
  function clear() {
    setStatus("all");
    setOrigin("all");
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
      Number(status !== "all") + Number(origin !== "all") + Number(pinned),
    clear,
    reset() {
      setSearch("");
      clear();
    },
  };
}

export type MemoryFilterState = ReturnType<typeof useMemoryFilters>;
