import { pluginContract, pluginMethod } from "@flame/runtime-client";
import type {
  PluginContribution,
  PluginInstallation,
} from "@flame/runtime-client/wire";
import {
  InvalidPresentation,
  type CapturedBinding,
  type OperationRequest,
} from "../contracts/document";

interface Callable {
  installation: PluginInstallation;
  contribution: PluginContribution;
}

export class OperationCatalog {
  readonly #callables: readonly Callable[];

  constructor(installations: readonly PluginInstallation[]) {
    this.#callables = structuredClone(installations).flatMap((installation) => {
      if (
        !installation.enabled ||
        installation.presentation !== "admitted" ||
        installation.realization.type !== "available"
      ) {
        return [];
      }
      const unavailable = new Set([
        ...installation.disabledServers,
        ...(installation.realization.unavailableBackends ?? []),
      ]);
      return installation.selected.contributions
        .filter((contribution) => !unavailable.has(contribution.server))
        .map((contribution) => ({ installation, contribution }));
    });
  }

  describe(installationId?: string) {
    return this.#callables
      .filter(
        ({ installation }) =>
          installationId === undefined || installation.id === installationId,
      )
      .map(({ installation, contribution }) => {
        const contract = pluginContract(contribution.contract);
        return {
          installationId: installation.id,
          name: installation.selected.name,
          contribution: contribution.id,
          contract: contribution.contract,
          reference: contract.reference,
          methods: contract.methods.map((value) => {
            if (
              value === null ||
              typeof value !== "object" ||
              !("name" in value) ||
              typeof value.name !== "string"
            ) {
              throw new Error(
                "genui: admitted contract contains an invalid method",
              );
            }
            return {
              name: value.name,
              ...pluginMethod({
                contract: contribution.contract,
                method: value.name,
              }),
            };
          }),
        };
      });
  }

  capture(requests: readonly OperationRequest[]): CapturedBinding[] {
    return requests.map((request) => {
      const matches = this.#callables.filter(
        ({ installation, contribution }) =>
          installation.id === request.installationId &&
          contribution.id === request.contribution,
      );
      if (matches.length !== 1) {
        throw new InvalidPresentation(
          "operation requires one available admitted contribution: " +
            request.id,
        );
      }
      const { installation, contribution } = matches[0]!;
      const binding = {
        installationId: installation.id,
        digest: installation.selected.digest,
        contribution: contribution.id,
        contract: contribution.contract,
        method: request.method,
      };
      let method;
      try {
        method = pluginMethod(binding);
      } catch (cause) {
        if (cause instanceof TypeError) {
          throw new InvalidPresentation(
            "operation does not name an admitted method: " + request.id,
            cause,
          );
        }
        throw cause;
      }
      const target =
        method.target === "installation"
          ? { scope: "installation", reference: installation.id }
          : request.target;
      if (
        !target ||
        target.scope !== method.target ||
        (request.target !== undefined &&
          (request.target.scope !== target.scope ||
            request.target.reference !== target.reference))
      ) {
        throw new InvalidPresentation(
          "operation target differs from its admitted method: " + request.id,
        );
      }
      return { id: request.id, binding: { ...binding, target: { ...target } } };
    });
  }
}
