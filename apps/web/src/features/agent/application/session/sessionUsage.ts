import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { nanoid } from "nanoid";
import { type Query } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import {
  type AgentRuntimeGateway,
  type AgentSessionUsage,
} from "@/features/agent/application/runtimeGateway";
export function createAgentSessionUsage() {
  const scope = ["usage.session", nanoid()] as const;
  const queryKey = (sessionId?: string) =>
    sessionId === undefined ? scope : ([...scope, sessionId] as const);
  const ownsQuery = (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] === scope[0] && query.queryKey[1] === scope[1];

  class AgentSessionUsageOwner {
    readonly #lifetime = new AbortController();
    #retired = false;

    private constructor(private readonly gateway: AgentRuntimeGateway) {}

    static install(gateway: AgentRuntimeGateway): AgentSessionUsageOwner {
      const owner = new AgentSessionUsageOwner(gateway);
      agentSessionUsagePublication.publish(owner, (predecessor) => predecessor.#retire());
      owner.#replaceCachedWriter();
      return owner;
    }

    static current(): AgentSessionUsageOwner {
      const owner = agentSessionUsagePublication.current();
      if (!owner) throw new GenerationRetiredError("agent_session_usage_owner");
      return owner;
    }

    async load(sessionId: string, querySignal: AbortSignal): Promise<AgentSessionUsage> {
      this.#assertCurrent();
      const attempt = AbortSignal.any([querySignal, this.#lifetime.signal]);
      const usage = await this.gateway.loadSessionUsage(sessionId, attempt);
      this.#assertCurrent();
      return usage;
    }

    dispose(): void {
      if (this.#retired) return;
      const current = agentSessionUsagePublication.owns(this);
      const cachedWriters = current ? this.#cachedWriters() : undefined;
      agentSessionUsagePublication.withdraw(this);
      this.#retire();
      if (cachedWriters && cachedWriters.size > 0) {
        void queryClient.cancelQueries({ predicate: (query) => cachedWriters.has(query) });
      }
    }

    #replaceCachedWriter(): void {
      const cachedWriters = this.#cachedWriters();
      if (cachedWriters.size === 0) return;
      const ownedWriter = (query: Query) => cachedWriters.has(query);
      void queryClient.cancelQueries({ predicate: ownedWriter }).then(() => {
        if (this.#retired || !agentSessionUsagePublication.owns(this)) return;
        void queryClient.resetQueries({ predicate: ownedWriter });
      });
    }

    #cachedWriters(): ReadonlySet<Query> {
      return new Set(queryClient.getQueryCache().findAll({ predicate: ownsQuery }));
    }

    #assertCurrent(): void {
      if (this.#retired || !agentSessionUsagePublication.owns(this)) {
        throw new GenerationRetiredError("agent_session_usage_owner");
      }
    }

    #retire(): void {
      if (this.#retired) return;
      this.#retired = true;
      this.#lifetime.abort(new GenerationRetiredError("agent_session_usage_owner"));
    }
  }

  const agentSessionUsagePublication = createPublicationSlot<AgentSessionUsageOwner>();

  function useAgentSessionUsage(sessionId: string | undefined) {
    return useQuery({
      queryKey: queryKey(sessionId),
      queryFn: ({ signal }) => AgentSessionUsageOwner.current().load(sessionId ?? "", signal),
      enabled: Boolean(sessionId),
    });
  }

  return {
    queryKey,
    ownsQuery,
    replace: AgentSessionUsageOwner.install,
    useAgentSessionUsage,
    dispose() {
      agentSessionUsagePublication.current()?.dispose();
    },
  };
}
export type AgentSessionUsageOwner = ReturnType<
  ReturnType<typeof createAgentSessionUsage>["replace"]
>;
