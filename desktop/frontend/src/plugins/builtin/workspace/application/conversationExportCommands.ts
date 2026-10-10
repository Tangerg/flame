import type { CommandSpec } from "@/plugins/sdk";

type CommandRun = CommandSpec["run"];

export interface ConversationExportCommandHandlers {
  exportMarkdown: CommandRun;
  exportJson: CommandRun;
  exportTrajectory: CommandRun;
  importJson: CommandRun;
}

export function conversationExportCommands(
  handlers: ConversationExportCommandHandlers,
): CommandSpec[] {
  return [
    {
      id: "chat.export.markdown",
      label: "convExport.markdown",
      run: handlers.exportMarkdown,
    },
    {
      id: "chat.export.json",
      label: "convExport.json",
      run: handlers.exportJson,
    },
    {
      id: "chat.export.trajectory",
      label: "convExport.trajectory",
      run: handlers.exportTrajectory,
    },
    {
      id: "chat.import.json",
      label: "convExport.import",
      run: handlers.importJson,
    },
  ];
}
