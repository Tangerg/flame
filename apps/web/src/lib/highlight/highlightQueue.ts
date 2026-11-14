import type { HighlightRequest, HighlightResponse } from "./shiki.worker";

type Job = {
  owner: symbol;
  request: Omit<HighlightRequest, "grammars">;
  resolve: (result: Extract<HighlightResponse, { status: "ready" }>) => void;
  reject: (error: unknown) => void;
  release: () => void;
  retired: boolean;
};

function retired(): DOMException {
  return new DOMException("highlight request retired", "AbortError");
}

export function createHighlightQueue(
  createWorker: () => Promise<Worker>,
  loadLanguage: (lang: string) => Promise<HighlightRequest["grammars"]>,
) {
  let worker: Worker | null = null;
  let starting: Promise<Worker> | null = null;
  let active: Job | null = null;
  let sequence = 0;
  const pending = new Map<symbol, Job>();

  function rejectJob(job: Job, error: unknown) {
    job.release();
    if (!job.retired) job.reject(error);
    job.retired = true;
  }

  function fail(error: unknown) {
    worker?.terminate();
    worker = null;
    starting = null;
    if (active) rejectJob(active, error);
    active = null;
    for (const job of pending.values()) rejectJob(job, error);
    pending.clear();
  }

  function acquire(): Promise<Worker> {
    if (worker) return Promise.resolve(worker);
    if (starting) return starting;
    const attempt: Promise<Worker> = Promise.resolve()
      .then(createWorker)
      .then((created) => {
        if (starting !== attempt) {
          created.terminate();
          throw retired();
        }
        worker = created;
        created.onmessage = (event: MessageEvent<HighlightResponse>) => {
          if (worker !== created) return;
          const job = active;
          const response = event.data;
          if (!job || response.id !== job.request.id) {
            fail(new Error("highlight worker returned an unexpected request"));
            return;
          }
          active = null;
          job.release();
          if (!job.retired) {
            if (response.status === "ready") job.resolve(response);
            else job.reject(new Error(response.error.message, { cause: response.error.details }));
          }
          pump();
        };
        created.onerror = (event) => {
          if (worker !== created) return;
          event.preventDefault();
          fail(event.error ?? new Error(event.message));
        };
        created.onmessageerror = () => {
          if (worker === created) fail(new Error("highlight worker response could not be decoded"));
        };
        return created;
      })
      .finally(() => {
        if (starting === attempt) starting = null;
      });
    starting = attempt;
    return attempt;
  }

  function pump() {
    if (active || pending.size === 0) return;
    const [owner, job] = pending.entries().next().value!;
    pending.delete(owner);
    active = job;
    void acquire().then(
      async (ready) => {
        if (active !== job) return;
        if (job.retired) {
          active = null;
          pump();
          return;
        }
        let grammars: HighlightRequest["grammars"];
        try {
          grammars = await loadLanguage(job.request.lang);
        } catch (error) {
          if (active !== job) return;
          active = null;
          rejectJob(job, error);
          pump();
          return;
        }
        if (active !== job) return;
        if (job.retired) {
          active = null;
          pump();
          return;
        }
        try {
          ready.postMessage({ ...job.request, grammars } satisfies HighlightRequest);
        } catch (error) {
          fail(error);
        }
      },
      (error: unknown) => {
        if (active === job) fail(error);
      },
    );
  }

  return {
    request(
      owner: symbol,
      input: Omit<HighlightRequest, "id" | "grammars">,
      signal: AbortSignal,
    ): Promise<Extract<HighlightResponse, { status: "ready" }>> {
      if (signal.aborted) return Promise.reject(retired());
      const superseded = pending.get(owner);
      if (superseded) {
        pending.delete(owner);
        rejectJob(superseded, retired());
      }
      if (active?.owner === owner) rejectJob(active, retired());
      const result = Promise.withResolvers<Extract<HighlightResponse, { status: "ready" }>>();
      const job: Job = {
        owner,
        request: { id: ++sequence, ...input },
        resolve: result.resolve,
        reject: result.reject,
        release: () => signal.removeEventListener("abort", cancel),
        retired: false,
      };
      function cancel() {
        if (pending.get(owner) === job) pending.delete(owner);
        rejectJob(job, retired());
      }
      signal.addEventListener("abort", cancel, { once: true });
      // Each live reader contributes at most one latest waiting request. The single
      // in-flight job finishes in the Worker; retirement never publishes its result.
      pending.set(owner, job);
      pump();
      return result.promise;
    },
    dispose(): void {
      fail(retired());
    },
  };
}
