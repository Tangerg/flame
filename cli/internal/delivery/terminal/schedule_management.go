package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func schedulesDocument(schedules []protocol.Schedule) readerDocument {
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
		if scheduled.Enabled {
			status = "enabled"
		}
		metadata := []string{
			"id       " + scheduled.ID,
			"cron     " + scheduled.Cron,
			"status   " + status,
			"revision " + fmt.Sprint(scheduled.Revision),
			"created  " + scheduled.CreatedAt.Format(time.RFC3339),
		}
		if scheduled.Workspace != nil {
			metadata = append(metadata, "workspace "+scheduled.Workspace.Path)
		}
		if scheduled.Provider != "" {
			metadata = append(metadata, "model    "+scheduled.Provider+"/"+scheduled.Model)
		}
		if scheduled.ReasoningEffort != "" {
			metadata = append(metadata, "reasoning "+scheduled.ReasoningEffort)
		}
		if scheduled.NextRunAt != nil {
			metadata = append(metadata, "next     "+scheduled.NextRunAt.Format(time.RFC3339))
		}
		if scheduled.LastRunAt != nil {
			metadata = append(metadata, "last     "+scheduled.LastRunAt.Format(time.RFC3339))
		}
		sections = append(sections,
			ToolSection{Title: title, Style: toolSectionParagraph, Text: scheduled.Instructions, Links: true},
			ToolSection{Title: "Configuration", Style: toolSectionCode, Language: "text", Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: "Scheduled runs", Detail: fmt.Sprintf("%d configured", len(schedules)), Sections: sections}
}

func (a *app) OpenScheduleCreateForm() error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	a.openScheduleForm(scheduleFormCreate, protocol.Schedule{})
	return nil
}

func (a *app) EditSchedule(identity string) error {
	return a.loadSchedule(identity, "loading schedule to edit", func(scheduled protocol.Schedule) {
		a.openScheduleForm(scheduleFormUpdate, scheduled)
	})
}

func (a *app) SetScheduleEnabled(identity string, enabled bool) error {
	verb := "enabling"
	if !enabled {
		verb = "disabling"
	}
	return a.loadSchedule(identity, verb+" schedule", func(scheduled protocol.Schedule) {
		if scheduled.Enabled == enabled {
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			a.message("schedule is already " + state + " · " + scheduled.ID)
			return
		}
		request := protocol.UpdateScheduleRequest{ID: scheduled.ID, ExpectedRevision: scheduled.Revision, Enabled: &enabled}
		a.updateSchedule(request, verb+" schedule "+scheduled.ID)
	})
}

func (a *app) PrepareDeleteSchedule(identity string) error {
	return a.loadSchedule(identity, "loading schedule to delete", func(scheduled protocol.Schedule) {
		title := scheduled.Title
		if title == "" {
			title = scheduled.ID
		}
		a.confirmAction("Delete scheduled run", "Delete "+title+" ("+scheduled.ID+")?", "Delete permanently", func() {
			a.deleteSchedule(scheduled.ID)
		})
	})
}

func (a *app) RunScheduleNow(identity string) error {
	return a.loadSchedule(identity, "loading schedule to run", func(scheduled protocol.Schedule) {
		a.status.note("running schedule " + scheduled.ID)
		started := a.runApplicationOperation(scheduleOperation, false,
			func(ctx context.Context) (protocol.RunScheduleNowResponse, error) {
				return a.schedules.RunNow(ctx, scheduled.ID)
			},
			func(handle protocol.RunScheduleNowResponse, err error) {
				if err != nil {
					a.message("run schedule now failed: " + err.Error())
					return
				}
				a.message("schedule started · session " + handle.SessionID + " · run " + handle.RunID)
			},
		)
		if !started {
			a.message("another schedule operation is running")
		}
	})
}

func (a *app) loadSchedule(identity, label string, apply func(protocol.Schedule)) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("a schedule id, unique prefix, or unique title is required")
	}
	a.status.note(label)
	started := a.runOperation(scheduleOperation, false,
		func(ctx context.Context) (protocol.Schedule, error) {
			schedules, err := a.schedules.Schedules(ctx)
			if err != nil {
				return protocol.Schedule{}, err
			}
			return resolveSchedule(schedules, identity)
		},
		func(scheduled protocol.Schedule, err error) {
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

func resolveSchedule(schedules []protocol.Schedule, identity string) (protocol.Schedule, error) {
	for _, scheduled := range schedules {
		if scheduled.ID == identity {
			return scheduled, nil
		}
	}
	matches := make([]protocol.Schedule, 0, 1)
	for _, scheduled := range schedules {
		if strings.HasPrefix(scheduled.ID, identity) || (scheduled.Title != "" && scheduled.Title == identity) {
			matches = append(matches, scheduled)
		}
	}
	switch len(matches) {
	case 0:
		return protocol.Schedule{}, errors.New("schedule not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return protocol.Schedule{}, errors.New("schedule identity is ambiguous; use the full id")
	}
}

func (a *app) createSchedule(request protocol.CreateScheduleRequest) {
	presentation := a.session.context
	a.status.note("creating schedule")
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (protocol.Schedule, error) { return a.schedules.Create(ctx, request) },
		func(created protocol.Schedule, err error) {
			if err != nil {
				a.message("create schedule failed: " + err.Error())
				return
			}
			a.reportScheduleMutation("schedule created · "+created.ID, presentation)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) updateSchedule(request protocol.UpdateScheduleRequest, label string) {
	presentation := a.session.context
	a.status.note(label)
	started := a.runApplicationOperation(scheduleOperation, false,
		func(ctx context.Context) (protocol.Schedule, error) { return a.schedules.Update(ctx, request) },
		func(updated protocol.Schedule, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.reportScheduleMutation("schedule updated · "+updated.ID, presentation)
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
		func(ctx context.Context) (string, error) { return id, a.schedules.Delete(ctx, id) },
		func(deleted string, err error) {
			if err != nil {
				a.message("delete schedule failed: " + err.Error())
				return
			}
			a.reportScheduleMutation("schedule deleted · "+deleted, presentation)
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) reportScheduleMutation(message string, presentation *sessionContextLease) {
	a.message(message)
	if a.session.context.current(presentation) {
		a.ShowSchedules()
	}
}
