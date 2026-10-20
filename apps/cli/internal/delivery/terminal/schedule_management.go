package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/installedcommand"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/schedule"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowSchedules() {
	if a.schedules == nil {
		a.message("this runtime composition has no schedule service")
		return
	}
	a.executeRuntimeReaderQuery(a.schedulesReaderQuery())
}

func (a *app) schedulesReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading schedules",
		mode:   runtimeReaderSchedules,
		read: func(ctx context.Context) (readerDocument, error) {
			schedules, err := a.schedules.Schedules(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return schedulesDocument(schedules), nil
		},
	}
}

func schedulesDocument(schedules []schedule.CreateV1Output) readerDocument {
	if len(schedules) == 0 {
		return paragraphDocument("Scheduled runs", "none configured", []string{"No scheduled runs are configured."})
	}
	sections := make([]ToolSection, 0, len(schedules)*2)
	for _, scheduled := range schedules {
		title := scheduled.Title
		if title == "" {
			title = "Untitled schedule"
		}
		status := "disabled"
		if !scheduled.NextRunAt.IsZero() {
			status = "enabled"
		}
		metadata := []string{
			"id       " + scheduled.ID,
			"cron     " + scheduled.Cron,
			"status   " + status,
			"revision " + fmt.Sprint(scheduled.Revision),
			"created  " + scheduled.CreatedAt.Format(time.RFC3339),
		}
		if scheduled.CWD != "" {
			metadata = append(metadata, "workspace "+scheduled.CWD)
		}
		if scheduled.ModelSelection.Provider != "" {
			metadata = append(metadata, "model    "+scheduled.ModelSelection.Provider+"/"+scheduled.ModelSelection.Model)
		}
		if scheduled.ModelSelection.ReasoningEffort != "" {
			metadata = append(metadata, "reasoning "+scheduled.ModelSelection.ReasoningEffort)
		}
		if !scheduled.NextRunAt.IsZero() {
			metadata = append(metadata, "next     "+scheduled.NextRunAt.Format(time.RFC3339))
		}
		if !scheduled.LastRunAt.IsZero() {
			metadata = append(metadata, "last     "+scheduled.LastRunAt.Format(time.RFC3339))
		}
		sections = append(sections,
			ToolSection{Title: title, Style: toolSectionParagraph, Text: scheduled.Instructions, Links: true},
			ToolSection{Title: "Configuration", Style: toolSectionCode, Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: "Scheduled runs", Detail: fmt.Sprintf("%d configured", len(schedules)), Sections: sections}
}

func (a *app) OpenScheduleCreateForm() error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand("schedule-create"); pending {
			a.createSchedule(schedule.CreateV1Input{})
			return nil
		}
	}
	a.openScheduleForm(scheduleFormCreate, schedule.CreateV1Output{})
	return nil
}

func (a *app) EditSchedule(identity string) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand(scheduleUpdateIntent(identity)); pending {
			a.updateSchedule(schedule.UpdateV1Input{ID: identity}, "reconciling schedule "+identity)
			return nil
		}
	}
	return a.loadSchedule(identity, "loading schedule to edit", func(scheduled schedule.CreateV1Output) {
		a.openScheduleForm(scheduleFormUpdate, scheduled)
	})
}

func (a *app) SetScheduleEnabled(identity string, enabled bool) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand(scheduleUpdateIntent(identity)); pending {
			a.updateSchedule(schedule.UpdateV1Input{ID: identity}, "reconciling schedule "+identity)
			return nil
		}
	}
	verb := "enabling"
	if !enabled {
		verb = "disabling"
	}
	return a.loadSchedule(identity, verb+" schedule", func(scheduled schedule.CreateV1Output) {
		if !scheduled.NextRunAt.IsZero() == enabled {
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			a.message("schedule is already " + state + " · " + scheduled.ID)
			return
		}
		request := schedule.UpdateV1Input{ID: scheduled.ID, ExpectedRevision: scheduled.Revision, Enabled: &enabled}
		a.updateSchedule(request, verb+" schedule "+scheduled.ID)
	})
}

func (a *app) PrepareDeleteSchedule(identity string) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand(scheduleDeletionIntent(identity)); pending {
			a.deleteSchedule(identity)
			return nil
		}
	}
	return a.loadSchedule(identity, "loading schedule to delete", func(scheduled schedule.CreateV1Output) {
		title := scheduled.Title
		if title == "" {
			title = scheduled.ID
		}
		a.confirmAction("Delete scheduled run", "Delete "+title+" ("+scheduled.ID+")? Already claimed runs may still start.", "Delete permanently", func() {
			a.deleteSchedule(scheduled.ID)
		})
	})
}

func (a *app) RunScheduleNow(identity string) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand(scheduleRunIntent(identity)); pending {
			a.runSchedule(identity)
			return nil
		}
	}
	return a.loadSchedule(identity, "loading schedule to run", func(scheduled schedule.CreateV1Output) {
		a.runSchedule(scheduled.ID)
	})
}

func (a *app) runSchedule(id string) {
	a.status.note("running schedule " + id)
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (conversation.RunReference, error) {
			return installedcommand.Execute(ctx, a.schedules, a.workbench, scheduleRunIntent(id), a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.schedules.PrepareRunNow(ctx, id)
			}, a.schedules.DecodeRunNow)
		},
		func(handle conversation.RunReference, err error) {
			if err != nil {
				message := "run schedule now failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand(scheduleRunIntent(id)); pending {
						message += " · retry /schedule-run " + id + " to reconcile the original command"
					}
				}
				a.message(message)
				return
			}
			a.message("schedule started · session " + handle.SessionID + " · run " + handle.RunID)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func scheduleRunIntent(id string) string { return "schedule-run:" + id }

func (a *app) loadSchedule(identity, label string, apply func(schedule.CreateV1Output)) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("a schedule id, unique prefix, or unique title is required")
	}
	a.status.note(label)
	started := a.runOperation(scheduleOperation, false,
		func(ctx context.Context) (schedule.CreateV1Output, error) {
			schedules, err := a.schedules.Schedules(ctx)
			if err != nil {
				return schedule.CreateV1Output{}, err
			}
			return resolveSchedule(schedules, identity)
		},
		func(scheduled schedule.CreateV1Output, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			apply(scheduled)
		},
	)
	if !started {
		return errors.New("another schedule operation is running")
	}
	return nil
}

func resolveSchedule(schedules []schedule.CreateV1Output, identity string) (schedule.CreateV1Output, error) {
	for _, scheduled := range schedules {
		if scheduled.ID == identity {
			return scheduled, nil
		}
	}
	matches := make([]schedule.CreateV1Output, 0, 1)
	for _, scheduled := range schedules {
		if strings.HasPrefix(scheduled.ID, identity) || (scheduled.Title != "" && scheduled.Title == identity) {
			matches = append(matches, scheduled)
		}
	}
	switch len(matches) {
	case 0:
		return schedule.CreateV1Output{}, errors.New("schedule not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return schedule.CreateV1Output{}, errors.New("schedule identity is ambiguous; use the full id")
	}
}

func (a *app) createSchedule(request schedule.CreateV1Input) {
	presentation := a.session.context
	a.status.note("creating schedule")
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (schedule.CreateV1Output, error) {
			return installedcommand.Execute(ctx, a.schedules, a.workbench, "schedule-create", a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.schedules.PrepareCreate(ctx, request)
			}, a.schedules.DecodeCreate)
		},
		func(created schedule.CreateV1Output, err error) {
			if err != nil {
				message := "create schedule failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand("schedule-create"); pending {
						message += " · retry /schedule-create to reconcile the original command"
					}
				}
				a.message(message)
				return
			}
			a.reportScheduleMutation("schedule created · "+created.ID, presentation)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) updateSchedule(request schedule.UpdateV1Input, label string) {
	presentation := a.session.context
	a.status.note(label)
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (schedule.UpdateV1Output, error) {
			return installedcommand.Execute(ctx, a.schedules, a.workbench, scheduleUpdateIntent(request.ID), a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.schedules.PrepareUpdate(ctx, request)
			}, func(reply *protocol.CallPluginResponse) (schedule.UpdateV1Output, error) {
				return a.schedules.DecodeUpdate(request.ID, reply)
			})
		},
		func(updated schedule.UpdateV1Output, err error) {
			if err != nil {
				message := label + " failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand(scheduleUpdateIntent(request.ID)); pending {
						message += " · retry /schedule-edit " + request.ID + " to reconcile the original command"
					}
				}
				a.message(message)
				return
			}
			message := "schedule updated · " + updated.ID
			if updated.NextRunAt.IsZero() {
				message += " · already claimed runs may still start"
			}
			a.reportScheduleMutation(message, presentation)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) deleteSchedule(id string) {
	presentation := a.session.context
	a.status.note("deleting schedule " + id)
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (string, error) {
			_, err := installedcommand.Execute(ctx, a.schedules, a.workbench, scheduleDeletionIntent(id), a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.schedules.PrepareDelete(ctx, id)
			}, func(*protocol.CallPluginResponse) (struct{}, error) { return struct{}{}, nil })
			return id, err
		},
		func(deleted string, err error) {
			if err != nil {
				message := "delete schedule failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand(scheduleDeletionIntent(id)); pending {
						message += " · retry /schedule-delete " + id + " to reconcile the original command"
					}
				}
				a.message(message)
				return
			}
			a.reportScheduleMutation("schedule deleted · "+deleted, presentation)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func scheduleDeletionIntent(id string) string { return "schedule-delete:" + id }

func (a *app) reportScheduleMutation(message string, presentation *sessionContextLease) {
	a.message(message)
	if a.session.context.current(presentation) {
		a.ShowSchedules()
	}
}

func scheduleUpdateIntent(id string) string { return "schedule-edit:" + id }
