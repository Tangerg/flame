// @vitest-environment happy-dom
import { act, useEffect } from "react";
import { describe, expect, it } from "vitest";
import { Lifetime, Service } from "@qimu/sdk";
import type { Use } from "@qimu/browser";
import type { ObjectRef } from "@qimu/sdk/protocol";
import { mount, useLifetime } from "./index.ts";

describe("React adapter", () => {
  it("renders a native component while keeping async work owned after unmount", async () => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    let finish!: () => void;
    let unmounted = false;
    function Preview({ title }: { title: string }) {
      const lifetime = useLifetime();
      useEffect(() => {
        void lifetime.run(
          () =>
            new Promise<void>((resolve) => {
              finish = resolve;
            }),
        );
        return () => {
          unmounted = true;
        };
      }, [lifetime]);
      return <div>{title}</div>;
    }
    const contract = new Service<typeof Preview>({
      name: "documents.react-preview",
      version: "1",
      digest: "a".repeat(64),
      kind: "service",
      mode: "native",
    });
    const reference: ObjectRef = {
      objectId: "object",
      export: {
        instanceId: "instance",
        name: "preview",
        contract: contract.ref,
      },
    };
    const lifetime = new Lifetime("component-use");
    const use: Use = {
      id: "use",
      objects: [reference],
      lifetime,
      with: (_reference, _contract, work) =>
        lifetime.run((signal) => work(Preview as never, signal)),
      close: (signal) => lifetime.close(signal),
      wait: (signal) => lifetime.wait(signal),
    };
    const container = document.createElement("div");
    document.body.append(container);
    let mounted!: Awaited<ReturnType<typeof mount<{ title: string }>>>;
    await act(async () => {
      mounted = await mount(
        use,
        reference,
        contract,
        container,
        { title: "Document" },
        (error) => {
          throw error;
        },
      );
    });
    expect(container.textContent).toBe("Document");
    let closed = false;
    let closing!: Promise<unknown>;
    await act(async () => {
      closing = mounted.close().then(() => {
        closed = true;
      });
      await Promise.resolve();
    });
    expect(unmounted).toBe(true);
    expect(closed).toBe(false);
    expect(container.textContent).toBe("");
    finish();
    await closing;
    expect(closed).toBe(true);
    await expect(mounted.render({ title: "Retired" })).rejects.toThrow();
    container.remove();
  });
});
