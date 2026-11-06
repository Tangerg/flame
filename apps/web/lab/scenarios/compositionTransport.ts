import { stringifyJSON } from "@frame/sdk/json";
import type { FlameClient, MutationPromise } from "@flame/runtime-client";
import type {
  PluginBrowserSession,
  PluginClientComponent,
  PluginInstallation,
} from "@flame/runtime-client/wire";
import { componentExport } from "@flame/component-sdk/runtime-profile";
import {
  PROTOCOL,
  type CloseProof,
  type LocalPlan,
  type ReadyReport,
  type UseGrant,
} from "@frame/sdk/protocol";

function mutation<T>(
  create: () => Promise<T>,
  key: string = crypto.randomUUID(),
): MutationPromise<T> {
  return Object.assign(Promise.resolve().then(create), {
    idempotencyKey: key,
    retry: () => mutation(create, key),
  });
}

export function createCompositionTransport(
  installation: PluginInstallation,
  entry: PluginClientComponent,
) {
  const session: PluginBrowserSession = {
    authority: { id: "lab-composition", epoch: "1" },
    realm: { id: "lab-document", epoch: "1" },
    credential: "recorded-lab-credential",
    controlEndpoint: new URL("/lab/composition-control", location.origin).href,
    maximumMessageBytes: 1 << 20,
    proofCapacity: 128,
    pollIntervalMilliseconds: 10,
    preparationCapacity: 64,
    componentContract: {
      name: "io.github.tangerg.flame/component",
      version: "1",
      digest: "b".repeat(64),
      kind: "service",
      mode: "native",
    },
  };
  const plan: LocalPlan = {
    protocol: PROTOCOL,
    approvalToken: "recorded-composition-plan",
    authority: session.authority,
    realm: session.realm,
    execution: { id: "composition", generation: "1" },
    scopeId: "composition",
    scopeGeneration: "1",
    units: [
      {
        instance: {
          id: "composition-frame",
          generation: "1",
          source: {
            installationId: installation.id,
            release: installation.selected.digest,
            unitId: "component/frame",
          },
          configuration: "c".repeat(64),
        },
        configurationJson: JSON.stringify({
          export: entry.export,
          component: { id: entry.id },
          sharedModules: [],
        }),
        profile: "browser-js",
        module: { path: entry.entry, digest: "d".repeat(64) },
        imports: [],
        exports: [{ name: componentExport, contract: session.componentContract }],
      },
    ],
  };
  const prepared = Promise.withResolvers<ReadyReport>();
  const grants = new Map<string, UseGrant>();
  let requested = false;
  let ready: ReadyReport | undefined;
  let sealed = false;
  const client = {
    plugins: {
      openBrowserRealm: () => mutation(async () => session),
      acquireComponent: (request) =>
        mutation(async () => {
          if (sealed) throw new Error("composition fixture realm is closed");
          if (
            request.installationId !== installation.id ||
            request.digest !== installation.selected.digest ||
            request.componentId !== entry.id
          )
            throw new Error("composition fixture cannot admit an unrecorded component");
          requested = true;
          const report = await prepared.promise;
          const grant: UseGrant = {
            id: request.operationId,
            operationId: request.operationId,
            ...(request.parentUseId ? { parentUseId: request.parentUseId } : {}),
            approvalToken: `use:${request.operationId}`,
            authority: plan.authority,
            realm: plan.realm,
            execution: plan.execution,
            scopeId: plan.scopeId,
            scopeGeneration: plan.scopeGeneration,
            objects: report.objects,
          };
          grants.set(grant.id, grant);
          return { ...structuredClone(grant), objects: [...grant.objects] };
        }, request.operationId),
      closeBrowserRealm: () => mutation(async () => void (sealed = true)),
      openResources: async () => {
        const closed = Promise.withResolvers<void>();
        return {
          result: { token: "baseline", entry: entry.entry },
          closed: closed.promise,
          close: async () => closed.resolve(),
        };
      },
      resolvePoint: async (request) => ({
        data: (request.order ?? []).map((member, index) => ({
          ...member,
          digest: installation.selected.digest,
          value: JSON.stringify({ component: entry.id, props: { label: `Frame ${index + 1}` } }),
        })),
      }),
    } satisfies Pick<
      FlameClient["plugins"],
      | "openBrowserRealm"
      | "acquireComponent"
      | "closeBrowserRealm"
      | "openResources"
      | "resolvePoint"
    >,
  } as unknown as FlameClient;
  const originalFetch = window.fetch;
  // Only the recorded control endpoint is intercepted. Production Frame admission,
  // module evaluation, component composition and close proofs still execute.
  const fetchControl: typeof fetch = async (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url !== session.controlEndpoint) return originalFetch.call(window, input, init);
    if (new Headers(init?.headers).get("authorization") !== `Bearer ${session.credential}`)
      throw new Error("composition fixture control credential differs");
    const message = JSON.parse(String(init?.body)) as { method: string; input: unknown };
    let result: unknown = null;
    switch (message.method) {
      case "poll":
        result = {
          authority: session.authority,
          realm: session.realm,
          plans: requested && !ready && !sealed ? [plan] : [],
          fences: sealed ? [{ scopeId: plan.scopeId, scopeGeneration: plan.scopeGeneration }] : [],
        };
        break;
      case "admitPlan":
        result = {
          authority: plan.authority,
          realm: plan.realm,
          execution: plan.execution,
          decision: "admitted",
        };
        break;
      case "ready":
        ready = message.input as ReadyReport;
        prepared.resolve(ready);
        result = { decision: "accepted" };
        break;
      case "validateUse": {
        const grant = message.input as UseGrant;
        if (stringifyJSON(grant) !== stringifyJSON(grants.get(grant.id)))
          throw new Error("composition fixture received an unrecorded grant");
        break;
      }
      case "prove": {
        const proof = message.input as CloseProof;
        result = {
          authority: session.authority,
          closeId: proof.closeId,
          proofId: proof.proofId,
          recorded: true,
        };
        break;
      }
      default:
        throw new Error(`unmodelled composition control method: ${message.method}`);
    }
    return new Response(JSON.stringify(result), {
      headers: { "Content-Type": "application/json" },
    });
  };
  window.fetch = fetchControl;
  return {
    client,
    dispose() {
      if (window.fetch === fetchControl) window.fetch = originalFetch;
    },
  };
}
