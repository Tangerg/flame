export type ObservabilityTeardown = () => void | Promise<void>;

export function startObservability(
  initialize: () => Promise<ObservabilityTeardown>,
  reportFailure: (error: unknown) => void,
): ObservabilityTeardown {
  let disposed = false;
  let teardown: ObservabilityTeardown | null = null;
  let closing: Promise<void> | null = null;
  const close = (fn: ObservabilityTeardown) => {
    closing ??= Promise.resolve().then(fn).catch(reportFailure);
    return closing;
  };
  const initialized = Promise.resolve()
    .then(initialize)
    .then((fn) => {
      if (disposed) return close(fn);
      teardown = fn;
    })
    .catch(reportFailure);

  return async () => {
    disposed = true;
    const fn = teardown;
    teardown = null;
    if (fn) await close(fn);
    else if (closing) await closing;
    else await initialized;
  };
}
