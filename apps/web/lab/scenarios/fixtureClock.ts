/**
 * Makes every argument-free wall-clock read of `Date` follow `now`, so one recorded scene cannot
 * mix the fixture calendar with the system calendar. Explicit dates, parsing and the monotonic
 * performance, timer and animation-frame clocks stay native.
 */
export function installFixtureClock(now: () => number): () => void {
  const NativeDate = globalThis.Date;
  const FixtureDate: DateConstructor = new Proxy(NativeDate, {
    apply: () => new NativeDate(now()).toString(),
    construct: (target, args, newTarget) =>
      Reflect.construct(target, args.length === 0 ? [now()] : args, newTarget),
    get: (target, property, receiver) =>
      property === "now" ? now : Reflect.get(target, property, receiver),
  });
  globalThis.Date = FixtureDate;
  return () => {
    // A later installation owns the global; restoring here would discard its clock.
    if (globalThis.Date === FixtureDate) globalThis.Date = NativeDate;
  };
}
