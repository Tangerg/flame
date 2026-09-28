export type ConversationExportFormat = "md" | "json";

export type ConversationExportResult =
  { format: "md"; markdown?: string } | { format: "json"; artifact?: unknown };

interface ImportedConversation {
  id: string;
  title?: string;
}

export interface ConversationArchiveGateway {
  exportConversation(
    sessionId: string,
    format: ConversationExportFormat,
  ): Promise<ConversationExportResult>;
  exportTrajectory(sessionId: string): Promise<string>;
  importConversation(artifact: unknown): Promise<ImportedConversation>;
}
