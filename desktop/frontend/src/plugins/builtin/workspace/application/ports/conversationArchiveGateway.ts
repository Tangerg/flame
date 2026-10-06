export type ConversationExportFormat = "md" | "json";

interface ImportedConversation {
  id: string;
  title?: string;
}

export interface ConversationArchiveGateway {
  exportConversation(sessionId: string, format: ConversationExportFormat): Promise<string>;
  exportTrajectory(sessionId: string): Promise<string>;
  importConversation(artifact: unknown): Promise<ImportedConversation>;
}
