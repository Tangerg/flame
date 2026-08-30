// A foreign context spelling `openWorkspaceViewInDock("notifications")` takes an unchecked
// dependency on this context's id vocabulary: rename the view and the call still compiles
// while the click stops working. These functions are the checked form.
//
// Everything opened from the conversation lands in the DOCK — a click in the transcript
// must never cost the user the conversation. `settings` is the exception.

import { openWorkspaceView, openWorkspaceViewInDock } from "../application/navigation";

export function openTimelineView(): void {
  openWorkspaceViewInDock("timeline");
}

export function openDiagnosticsView(): void {
  openWorkspaceViewInDock("diagnostics");
}

export function openNotificationsView(): void {
  openWorkspaceViewInDock("notifications");
}

/** The settings view itself, with no pane in mind — the work index's entry.
 *  To land on a specific pane, use the settings pack's pane ids with
 *  `openWorkspaceSettingsPane`. */
export function openSettingsView(): void {
  openWorkspaceView("settings");
}

export function openDiffViewInDock(): void {
  openWorkspaceViewInDock("diff");
}
