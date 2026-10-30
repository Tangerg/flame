import { type SystemNotificationChannel } from "@flame/component-sdk";

function unavailable(): never {
  throw new Error("This fixture must not access system notifications");
}

export const unavailableComponentNotifications: SystemNotificationChannel = Object.freeze({
  applicationName: "Test application",
  authorization: unavailable,
  subscribe: unavailable,
  refreshAuthorization: unavailable,
  requestAuthorization: unavailable,
  send: unavailable,
});
