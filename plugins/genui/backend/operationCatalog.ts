import { pluginContract, pluginMethod } from "@flame/runtime-client";
import { stringifyJSON } from "@frame/sdk/json";
import type {
  PluginContribution,
  PluginInstallation,
} from "@flame/runtime-client/wire";
import {
  InvalidPresentation,
  type CapturedContribution,
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

  describe(installationId?: string, contributionId?: string) {
    return this.#callables
      .filter(
        ({ installation, contribution }) =>
          (installationId === undefined ||
            installation.id === installationId) &&
          (contributionId === undefined || contribution.id === contributionId),
      )
      .flatMap(({ installation, contribution }) => {
        const contract = pluginContract(contribution.contract);
        const identity = {
          installationId: installation.id,
          name: installation.selected.name,
          contribution: contribution.id,
          reference: contract.reference,
        };
        return contract.methods.map((value) => {
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
            ...identity,
            method: value.name,
            ...pluginMethod({
              contract: contribution.contract,
              method: value.name,
            }),
          };
        });
      });
  }

  method(installationId: string, contributionId: string, name: string) {
    const matches = this.#callables.filter(
      ({ installation, contribution }) =>
        installation.id === installationId &&
        contribution.id === contributionId,
    );
    if (matches.length !== 1)
      throw new InvalidPresentation(
        "operation requires one available admitted contribution",
      );
    const { installation, contribution } = matches[0]!;
    const contract = pluginContract(contribution.contract);
    try {
      pluginMethod({ contract: contribution.contract, method: name });
    } catch (cause) {
      if (cause instanceof TypeError)
        throw new InvalidPresentation(
          "operation does not name an admitted method",
          cause,
        );
      throw cause;
    }
    const method = contract.methods.find(
      (value) =>
        value !== null &&
        typeof value === "object" &&
        "name" in value &&
        value.name === name,
    );
    return {
      installationId: installation.id,
      contribution: contribution.id,
      reference: contract.reference,
      // Keep exact schema numbers in the owner's JSON codec. This is a selected
      // method projection; the accepted document captures the complete original contract.
      methodJSON: stringifyJSON(method),
    };
  }

  capture(requests: readonly OperationRequest[]): CapturedContribution[] {
    const captured = new Map<string, CapturedContribution>();
    for (const request of requests) {
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
      const contributionBinding = {
        installationId: installation.id,
        digest: installation.selected.digest,
        contribution: contribution.id,
        contract: contribution.contract,
      };
      let method;
      try {
        method = pluginMethod({
          ...contributionBinding,
          method: request.method,
        });
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
      captured.set(
        JSON.stringify([installation.id, contribution.id]),
        contributionBinding,
      );
    }
    return [...captured.values()];
  }
}
