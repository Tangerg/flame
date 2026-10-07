import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { t } from "@/lib/i18n";
import {
  getActiveSessionId,
  invalidateAgentSessions,
  rehydrateSessionView,
  selectAgentSession,
} from "@/plugins/builtin/agent/public/session";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { notifyError } from "@/plugins/sdk";
import { toast } from "sonner";
import type { ConversationArchiveGateway, ExportFormat } from "./ports/conversationArchiveGateway";
import type { FileTransferPort } from "./ports/fileTransfer";

function timestampForFilename(date: Date): string {
  return date.toISOString().replace(/[:.]/g, "-").slice(0, 19);
}

interface ExportMaterial {
  readonly filename: string;
  readonly content: string;
  readonly mime: string;
}

class ConversationArchiveGeneration {
  readonly #gateway: ConversationArchiveGateway;
  readonly #files: FileTransferPort;
  readonly #cohort = new RetirableTaskCohort(
    new GenerationRetiredError("conversation_archive_generation"),
  );
  #importOperation: Promise<void> | null = null;

  constructor(gateway: ConversationArchiveGateway, files: FileTransferPort) {
    this.#gateway = gateway;
    this.#files = files;
  }

  async export(format: ExportFormat): Promise<void> {
    try {
      const sessionId = getActiveSessionId();
      if (!sessionId) return;
      if (!runtimeCapability("sessionExport")) {
        notifyError(t("convExport.unsupported"), { source: "session" });
        return;
      }
      const content = await this.#cohort.run(() =>
        this.#gateway.exportConversation(sessionId, format),
      );
      this.#download({
        filename: `flame-${sessionId}-${timestampForFilename(new Date())}.${format}`,
        content,
        mime: format === "md" ? "text/markdown;charset=utf-8" : "application/json;charset=utf-8",
      });
    } catch (error) {
      if (!this.#cohort.retired) throw error;
    }
  }

  async exportTrajectory(): Promise<void> {
    try {
      const sessionId = getActiveSessionId();
      if (!sessionId) return;
      const stamp = timestampForFilename(new Date());
      const content = await this.#cohort.run(() => this.#gateway.exportTrajectory(sessionId));
      this.#download({
        filename: `flame-${sessionId}-trajectory-${stamp}.json`,
        content,
        mime: "application/json;charset=utf-8",
      });
    } catch (error) {
      if (!this.#cohort.retired) throw error;
    }
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
      if (!runtimeCapability("sessionExport")) {
        notifyError(t("convExport.unsupported"), { source: "import" });
        return;
      }
      const text = await this.#cohort.run(() => this.#files.pickText("application/json,.json"));
      if (text === null) return;

      let artifact: unknown;
      try {
        artifact = JSON.parse(text);
      } catch {
        this.#cohort.assertCurrent();
        notifyError(t("convExport.notJson"), { source: "import" });
        return;
      }

      const session = await this.#cohort.run(() => this.#gateway.importConversation(artifact));
      await this.#cohort.run(() => rehydrateSessionView(session.id));
      await this.#repairSessionList();
      this.#cohort.assertCurrent();
      selectAgentSession(session.id);
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

  #download(material: ExportMaterial): void {
    this.#cohort.assertCurrent();
    this.#files.download(material.filename, material.content, material.mime);
  }

  #releaseImport(operation: Promise<void>): void {
    if (this.#importOperation === operation) this.#importOperation = null;
  }
}

export interface ConversationArchiveOwnerDependencies {
  readonly gateway: ConversationArchiveGateway;
  readonly files: FileTransferPort;
}

export class ConversationArchiveOwner {
  readonly #files: FileTransferPort;
  #generation: ConversationArchiveGeneration;
  #disposed = false;

  private constructor(dependencies: ConversationArchiveOwnerDependencies) {
    this.#files = dependencies.files;
    this.#generation = new ConversationArchiveGeneration(dependencies.gateway, this.#files);
  }

  static install(dependencies: ConversationArchiveOwnerDependencies): ConversationArchiveOwner {
    const owner = new ConversationArchiveOwner(dependencies);
    conversationArchivePublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): ConversationArchiveOwner {
    const owner = conversationArchivePublication.current();
    if (!owner || owner.#disposed) throw new Error("Conversation archive owner is not installed");
    return owner;
  }

  export(format: ExportFormat): Promise<void> {
    return this.#generation.export(format);
  }

  exportTrajectory(): Promise<void> {
    return this.#generation.exportTrajectory();
  }

  importJson(): Promise<void> {
    return this.#generation.importJson();
  }

  replaceRuntimeGeneration(createGateway: () => ConversationArchiveGateway): void {
    if (this.#disposed || !conversationArchivePublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new ConversationArchiveGeneration(createGateway(), this.#files);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    conversationArchivePublication.withdraw(this);
  }
}

const conversationArchivePublication = createPublicationSlot<ConversationArchiveOwner>();

export function exportConversationMarkdown(): Promise<void> {
  return ConversationArchiveOwner.current().export("md");
}

export function exportConversationJson(): Promise<void> {
  return ConversationArchiveOwner.current().export("json");
}

export function exportSessionTrajectory(): Promise<void> {
  return ConversationArchiveOwner.current().exportTrajectory();
}

export function importConversationJson(): Promise<void> {
  return ConversationArchiveOwner.current().importJson();
}
