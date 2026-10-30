import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { NotificationsBadge } from "@/features/notifications/ui/NotificationsBadge";
export { createSystemNotificationsPlugin } from "@/features/notifications/systemNotificationRouting";

export const statusNotifications = definePlugin({
  name: "flame.builtin.status-notifications",
  setup(ctx) {
    contributeLayout(ctx, "sidebar.footer.status", {
      id: "notifications",
      order: 10,
      component: NotificationsBadge,
    });
  },
});
