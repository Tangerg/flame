import {
  createContext,
  type ReactNode,
  use,
  useLayoutEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import type { MessageActionMaterialization } from "@/plugins/builtin/chat/message-actions/public/messageActions";

type VisibleMaterialToken = symbol;
type VisibleMaterialGeneration = object;

interface VisibleProjection {
  generation: VisibleMaterialGeneration;
  settled: boolean;
}

export class MessageVisibleMaterialOwner {
  readonly identity: string;
  readonly #projections = new Map<VisibleMaterialToken, VisibleProjection>();
  readonly #listeners = new Set<() => void>();
  #revision: object = {};

  constructor(sessionId: string, messageId: string) {
    this.identity = JSON.stringify([sessionId, messageId]);
  }

  observe(
    token: VisibleMaterialToken,
    generation: VisibleMaterialGeneration,
    settled: boolean,
  ): void {
    const current = this.#projections.get(token);
    if (current?.generation === generation && current.settled === settled) return;
    const wasActive = this.#projectionsOwnActiveMaterial(generation);
    this.#projections.set(token, { generation, settled });
    if (wasActive !== this.#projectionsOwnActiveMaterial(generation)) this.#publish();
  }

  retire(token: VisibleMaterialToken): void {
    if (!this.#projections.delete(token)) return;
    this.#publish();
  }

  actionsMaterialization(
    source: MessageActionMaterialization,
    generation: VisibleMaterialGeneration,
  ): MessageActionMaterialization {
    return source === "active" || this.#projectionsOwnActiveMaterial(generation)
      ? "active"
      : "settled";
  }

  #projectionsOwnActiveMaterial(generation: VisibleMaterialGeneration): boolean {
    for (const projection of this.#projections.values()) {
      if (projection.generation !== generation || !projection.settled) return true;
    }
    return false;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  snapshot = (): object => this.#revision;

  #publish(): void {
    this.#revision = {};
    for (const listener of this.#listeners) listener();
  }
}

interface MessageVisibleMaterialContextValue {
  owner: MessageVisibleMaterialOwner;
  generation: VisibleMaterialGeneration;
}

const MessageVisibleMaterialContext = createContext<MessageVisibleMaterialContextValue | null>(
  null,
);

export function MessageVisibleMaterialProvider({
  owner,
  generation,
  children,
}: {
  owner: MessageVisibleMaterialOwner;
  generation: VisibleMaterialGeneration;
  children: ReactNode;
}) {
  const material = useMemo(() => ({ owner, generation }), [generation, owner]);
  return (
    <MessageVisibleMaterialContext.Provider value={material}>
      {children}
    </MessageVisibleMaterialContext.Provider>
  );
}

export function useVisibleActionMaterialization(
  owner: MessageVisibleMaterialOwner,
  source: MessageActionMaterialization,
  generation: VisibleMaterialGeneration,
): MessageActionMaterialization {
  useSyncExternalStore(owner.subscribe, owner.snapshot, owner.snapshot);
  return owner.actionsMaterialization(source, generation);
}

export function useVisibleTextMaterial(settled: boolean): void {
  const material = use(MessageVisibleMaterialContext);
  const [token] = useState<VisibleMaterialToken>(() => Symbol("visible-text-material"));

  useLayoutEffect(() => {
    if (!material) return;
    material.owner.observe(token, material.generation, settled);
  }, [material, settled, token]);

  const owner = material?.owner;
  useLayoutEffect(() => () => owner?.retire(token), [owner, token]);
}
