import { useEffect, useState } from "react";

export function useElapsedMillis(startedAt: number | null): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (startedAt === null) return;
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, [startedAt]);

  return startedAt === null ? 0 : Math.max(0, now - startedAt);
}
