import { service } from "dougong";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
export interface CommandSpec {
  id: string;
  label: string;
  combo?: string;
  run: (...args: unknown[]) => void | Promise<void>;
}

type ShortcutHandler = (event: KeyboardEvent) => void;

export interface ShortcutSpec {
  key: string;
  handler: ShortcutHandler;
  description?: string;
  allowInInputs?: boolean;
}

export interface CommandsService {
  execute(id: string, ...args: unknown[]): Promise<void>;
}

export const COMMANDS = service<CommandsService>("flame.shell.commands");

export function executeCommand(id: string, ...args: unknown[]): Promise<void> {
  const command = lookupExtensionByKey(COMMAND, id);
  if (!command) {
    console.warn(`[plugin] commands.execute("${id}"): no command registered`);
    return Promise.resolve();
  }
  return Promise.resolve(command.run(...args));
}
