import { createContext, useContext, type ReactNode } from "react";
import { type SystemNotifier } from "@/features/notifications/systemNotifier";
import { type createCompletionSoundStore } from "@/features/notifications/completionSound";
import { type createSystemNotificationsStore } from "@/features/notifications/systemNotifications";
export interface NotificationAccess {
  readonly notifier: SystemNotifier;
  readonly completionSound: ReturnType<typeof createCompletionSoundStore>;
  readonly systemNotifications: ReturnType<typeof createSystemNotificationsStore>;
}
const NotificationContext = createContext<NotificationAccess | null>(null);
export function NotificationProvider({
  value,
  children,
}: {
  value: NotificationAccess;
  children: ReactNode;
}) {
  return <NotificationContext value={value}>{children}</NotificationContext>;
}
export function useNotifications(): NotificationAccess {
  const value = useContext(NotificationContext);
  if (!value) throw new Error("NotificationProvider is missing");
  return value;
}
