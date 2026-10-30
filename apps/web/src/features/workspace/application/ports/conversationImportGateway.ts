interface ImportedConversation {
  id: string;
  title?: string;
}

export interface ConversationImportGateway {
  importConversation(artifact: unknown): Promise<ImportedConversation>;
}
