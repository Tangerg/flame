export type NotificationLevel = "info" | "warn" | "error";

export interface NotificationEntry {
  id: string;
  plugin: string;
  level: NotificationLevel;
  message: string;
  timestamp: number;
  dismissed?: boolean;
}
