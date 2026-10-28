import {
  createContext,
  createElement,
  useContext,
  type ComponentType,
  type ReactNode,
} from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  QimuError,
  type CloseOutcome,
  type Lifetime,
  type Contract,
} from "@qimu/sdk";
import { type Use } from "@qimu/browser";
import { type ObjectRef } from "@qimu/sdk/protocol";

const ResourceContext = createContext<Use | undefined>(undefined);

/** The explicit parent acquisition, when a renderer is nested in another Use. */
export function useCurrentUse(): Use | undefined {
  return useContext(ResourceContext);
}

/** Explicitly supplied by mount; no global Runtime or provider lookup exists. */
export function useLifetime(): Lifetime {
  const use = useCurrentUse();
  if (!use) throw new Error("component is outside its Qimu lifetime provider");
  return use.lifetime;
}

export interface Mounted<P> {
  render(props: P): Promise<void>;
  close(signal?: AbortSignal): Promise<CloseOutcome>;
  wait(signal?: AbortSignal): Promise<CloseOutcome>;
}

export async function mount<P extends object>(
  use: Use,
  reference: ObjectRef,
  contract: Contract<ComponentType<P>>,
  container: Element,
  props: P,
  onError: (error: unknown) => void,
  wrap: (children: ReactNode) => ReactNode = (children) => children,
): Promise<Mounted<P>> {
  if (contract.ref.mode !== "native")
    throw new QimuError(
      "contract_mismatch",
      "React components require a same-realm native contract",
    );
  const rendering = use.lifetime.branch("react-root");
  let root: Root | undefined = createRoot(container, {
    onUncaughtError(error) {
      use.lifetime.beginClose();
      onError(error);
    },
  });
  // Unmount can run while sibling async work drains. The parent Use still owns
  // that work and its provider dependencies until every branch has released.
  rendering.cleanup(() => {
    root?.unmount();
    root = undefined;
  }, "react-root");
  const render = async (next: P): Promise<void> =>
    use.with(reference, contract, (Component) => {
      rendering.assertOpen();
      root!.render(
        createElement(
          ResourceContext.Provider,
          { value: use },
          wrap(createElement(Component, next)),
        ),
      );
    });
  try {
    await render(props);
  } catch (error) {
    await use.close();
    throw error;
  }
  return Object.freeze({
    render,
    close: (signal?: AbortSignal) => use.close(signal),
    wait: (signal?: AbortSignal) => use.wait(signal),
  });
}
