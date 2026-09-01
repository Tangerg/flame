import { useEffect, useState } from "react";

/**
 * Wall-clock age of something still in flight, re-read once a second.
 *
 * This is the WAIT, not a measurement of work: it counts approval pauses, queueing and
 * anything else between the start and now, which is exactly what the person watching is
 * living through. A runtime-measured duration answers a different question and neither
 * substitutes for the other.
 */
export function useElapsedMillis(startedAt: number | null): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (startedAt === null) return;
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, [startedAt]);

  return startedAt === null ? 0 : Math.max(0, now - startedAt);
}
