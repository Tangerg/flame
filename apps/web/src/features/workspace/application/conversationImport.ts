import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { parseJSON } from "@frame/sdk/json";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { t } from "@/lib/i18n/index";

import { invalidateAgentSessions, sessionDisplayTitle } from "@/features/agent/public/session";

import { runtimeCapability } from "@/features/connection/public/capabilities";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { type ConversationImportGateway } from "@/features/workspace/application/ports/conversationImportGateway";
import { type FileTransferPort } from "@/features/workspace/application/ports/fileTransfer";

export function createConversationImport(
  agentAccess: AgentAccess,
  runtimeAccess: RuntimeAccess,
  notifications: NotificationFeed,
  files: FileTransferPort,
) {
  class ConversationImportTarget {
    readonly #gateway: ConversationImportGateway;
    readonly #cohort = new RetirableTaskCohort(
      new GenerationRetiredError("conversation_import_target"),
    );
    #importOperation: Promise<void> | null = null;

    constructor(gateway: ConversationImportGateway) {
      this.#gateway = gateway;
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
    }

    async #runImport(): Promise<void> {
      try {
        if (!runtimeCapability(runtimeAccess, "sessionExport")) {
          notifications.notify("error", t("convExport.unsupported"), { source: "import" });
          return;
        }
        let text: string | null;
        try {
          text = await this.#cohort.run(() => files.pickText("application/json,.json"));
        } catch (error) {
          if (this.#cohort.retired) return;
          console.error("[import] selected file read failed:", error);
          notifications.notify("error", t("convExport.readFailed"), { source: "import" });
          return;
        }
        if (text === null) return;

        let artifact: unknown;
        try {
          artifact = parseJSON(text);
        } catch {
          this.#cohort.assertCurrent();
          notifications.notify("error", t("convExport.notJson"), { source: "import" });
          return;
        }

        const session = await this.#cohort.run(() => this.#gateway.importConversation(artifact));
        this.#cohort.assertCurrent();
        agentAccess.selectAgentSession(session.id);
        invalidateAgentSessions([session.id]);
        notifications.show({
          level: "success",
          message: t("convExport.importSuccess", {
            title: sessionDisplayTitle(t, session.title),
          }),
        });
        void this.#cohort
          .run(() => agentAccess.synchronizeRewrittenSessionView(session.id))
          .catch((error: unknown) => {
            if (this.#cohort.retired) return;
            console.error("[import] imported Session view could not be refreshed:", error);
            notifications.notify("error", t("convExport.importRefreshFailed"), {
              source: "import",
            });
          });
      } catch (error) {
        if (this.#cohort.retired) return;
        console.error("[import] sessions.import failed:", error);
        this.#cohort.assertCurrent();
        notifications.notify("error", t("convExport.importFailed"), { source: "import" });
      }
    }

    #releaseImport(operation: Promise<void>): void {
      if (this.#importOperation === operation) this.#importOperation = null;
    }
  }

  let target: ConversationImportTarget | null = null;
  return {
    importConversationJson(): Promise<void> {
      if (!target) throw new Error("Conversation import has no Runtime target");
      return target.importJson();
    },
    replaceTarget(gateway: ConversationImportGateway): void {
      target?.retire();
      target = new ConversationImportTarget(gateway);
    },
    retireTarget(): void {
      target?.retire();
      target = null;
    },
  };
}
export type ConversationImport = ReturnType<typeof createConversationImport>;
