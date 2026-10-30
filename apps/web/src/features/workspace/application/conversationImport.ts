import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { parseJSON } from "@qimu/sdk/json";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { t } from "@/lib/i18n/index";

import { invalidateAgentSessions } from "@/features/agent/public/session";

import { runtimeCapability } from "@/features/connection/public/capabilities";
import { notifyError } from "@/features/notifications/public/notifications";
import { toast } from "sonner";
import { type ConversationImportGateway } from "@/features/workspace/application/ports/conversationImportGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";

export function createConversationImport(agentAccess: AgentAccess, runtimeAccess: RuntimeAccess) {
  class ConversationImportGeneration {
    readonly #gateway: ConversationImportGateway;
    readonly #files: FileTransferPort;
    readonly #cohort = new RetirableTaskCohort(
      new GenerationRetiredError("conversation_import_generation"),
    );
    #importOperation: Promise<void> | null = null;

    constructor(gateway: ConversationImportGateway, files: FileTransferPort) {
      this.#gateway = gateway;
      this.#files = files;
    }

    importJson(): Promise<void> {
      if (this.#importOperation) return this.#importOperation;
      const operation = this.#runImport();
      this.#importOperation = operation;
      void operation.then(
        () => this.#releaseImport(operation),
        () => this.#releaseImport(operation),
      );
      return operation;
    }

    retire(): void {
      this.#cohort.retire();
      this.#importOperation = null;
    }

    async #runImport(): Promise<void> {
      try {
        if (!runtimeCapability(runtimeAccess, "sessionExport")) {
          notifyError(t("convExport.unsupported"), { source: "import" });
          return;
        }
        let text: string | null;
        try {
          text = await this.#cohort.run(() => this.#files.pickText("application/json,.json"));
        } catch (error) {
          if (this.#cohort.retired) return;
          console.error("[import] selected file read failed:", error);
          notifyError(t("convExport.readFailed"), { source: "import" });
          return;
        }
        if (text === null) return;

        let artifact: unknown;
        try {
          artifact = parseJSON(text);
        } catch {
          this.#cohort.assertCurrent();
          notifyError(t("convExport.notJson"), { source: "import" });
          return;
        }

        const session = await this.#cohort.run(() => this.#gateway.importConversation(artifact));
        await this.#cohort.run(() => agentAccess.rehydrateSessionView(session.id));
        await this.#repairSessionList();
        this.#cohort.assertCurrent();
        agentAccess.selectAgentSession(session.id);
        toast.success(
          t("convExport.importSuccess", { title: session.title?.trim() || t("session.untitled") }),
        );
      } catch (error) {
        if (this.#cohort.retired) return;
        console.error("[import] sessions.import failed:", error);
        this.#cohort.assertCurrent();
        notifyError(t("convExport.importFailed"), { source: "import" });
      }
    }

    async #repairSessionList(): Promise<void> {
      try {
        await this.#cohort.run(() => invalidateAgentSessions());
      } catch (error) {
        if (this.#cohort.retired) throw error;
      }
    }

    #releaseImport(operation: Promise<void>): void {
      if (this.#importOperation === operation) this.#importOperation = null;
    }
  }

  interface ConversationImportOwnerDependencies {
    readonly gateway: ConversationImportGateway;
    readonly files: FileTransferPort;
  }

  class ConversationImportOwner {
    readonly #files: FileTransferPort;
    #generation: ConversationImportGeneration;
    #disposed = false;

    private constructor(dependencies: ConversationImportOwnerDependencies) {
      this.#files = dependencies.files;
      this.#generation = new ConversationImportGeneration(dependencies.gateway, this.#files);
    }

    static install(dependencies: ConversationImportOwnerDependencies): ConversationImportOwner {
      const owner = new ConversationImportOwner(dependencies);
      conversationImportPublication.publish(owner, (predecessor) => predecessor.dispose());
      return owner;
    }

    static current(): ConversationImportOwner {
      const owner = conversationImportPublication.current();
      if (!owner || owner.#disposed) throw new Error("Conversation import owner is not installed");
      return owner;
    }

    importJson(): Promise<void> {
      return this.#generation.importJson();
    }

    replaceRuntimeGeneration(createGateway: () => ConversationImportGateway): void {
      if (this.#disposed || !conversationImportPublication.owns(this)) return;
      const predecessor = this.#generation;
      this.#generation = new ConversationImportGeneration(createGateway(), this.#files);
      predecessor.retire();
    }

    dispose(): void {
      if (this.#disposed) return;
      this.#disposed = true;
      this.#generation.retire();
      conversationImportPublication.withdraw(this);
    }
  }

  const conversationImportPublication = createPublicationSlot<ConversationImportOwner>();

  function importConversationJson(): Promise<void> {
    return ConversationImportOwner.current().importJson();
  }

  return {
    install: ConversationImportOwner.install,
    importConversationJson,
  };
}
export type ConversationImport = ReturnType<typeof createConversationImport>;
export type ConversationImportOwner = ReturnType<ConversationImport["install"]>;
