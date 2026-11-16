import { createContext, useContext, type ReactNode } from "react";
import { type SystemNotifier } from "@/features/notifications/systemNotifier";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
export interface NotificationAccess {
  readonly notifier: SystemNotifier;
  readonly productName: string;
  readonly messages: NotificationFeed;
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
