type UnaryCutpoint = "beforeCommit" | "afterCommit";

export function faultUnaryResponse(
  method: string,
  cutpoint: UnaryCutpoint,
): { attempts: () => number; fetch: typeof fetch } {
  let attempts = 0;
  let injected = false;
  const faultFetch: typeof fetch = async (input, init) => {
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as { method?: string }) : {};
    if (body.method !== method) return globalThis.fetch(input, init);
    attempts += 1;
    if (injected) return globalThis.fetch(input, init);
    injected = true;
    if (cutpoint === "beforeCommit") {
      throw new Error("injected disconnect before Runtime admission");
    }

    const committed = await globalThis.fetch(input, init);
    await committed.arrayBuffer();
    const lostBody = new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.error(new Error("injected response body loss after Runtime commit"));
      },
    });
    return new Response(lostBody, {
      headers: committed.headers,
      status: committed.status,
      statusText: committed.statusText,
    });
  };
  return { attempts: () => attempts, fetch: faultFetch };
}

export function faultStreamOpening(method: string): {
  attempts: () => number;
  fetch: typeof fetch;
} {
  let attempts = 0;
  let injected = false;
  const faultFetch: typeof fetch = async (input, init) => {
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as { method?: string }) : {};
    if (body.method !== method) return globalThis.fetch(input, init);
    attempts += 1;
    const committed = await globalThis.fetch(input, init);
    if (injected) return committed;
    injected = true;
    if (!(committed.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
      throw new Error(`expected ${method} to return an event stream`);
    }
    await committed.body?.cancel("injected opening response loss after Runtime commit");
    const lostBody = new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.error(new Error("injected opening response loss after Runtime commit"));
      },
    });
    return new Response(lostBody, {
      headers: committed.headers,
      status: committed.status,
      statusText: committed.statusText,
    });
  };
  return { attempts: () => attempts, fetch: faultFetch };
}
