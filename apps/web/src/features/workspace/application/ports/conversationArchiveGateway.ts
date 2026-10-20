import { type ExportFormat } from "@flame/runtime-client/wire";
export type { ExportFormat };

interface ImportedConversation {
  id: string;
  title?: string;
}

export interface ConversationArchiveGateway {
  exportConversation(sessionId: string, format: ExportFormat): Promise<string>;
  exportTrajectory(sessionId: string): Promise<string>;
  importConversation(artifact: unknown): Promise<ImportedConversation>;
}
