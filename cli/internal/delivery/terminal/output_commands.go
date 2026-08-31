package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
)

type sessionOutputResult struct {
	sessionID string
	value     string
}

func (a *app) copyLastAssistant() error {
	sessionID := a.session.current.ID
	started := a.runOperation(sessionOutputOperation, false,
		func(ctx context.Context) (sessionOutputResult, error) {
			snapshot, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return sessionOutputResult{}, err
			}
			text, err := snapshot.LastAssistantText()
			return sessionOutputResult{sessionID: sessionID, value: text}, err
		},
		func(result sessionOutputResult, err error) {
			if err != nil {
				a.message("copy last response failed: " + err.Error())
				return
			}
			if a.session.current.ID != result.sessionID {
				a.message("copy canceled because the active session changed")
				return
			}
			if !a.loop.Clipboard().Copy(result.value) {
				a.message("the terminal host does not provide a clipboard")
				return
			}
			a.message("copied the last assistant response")
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func (a *app) exportSession(argument string) error {
	if err := a.requireRuntimeFeature(runtimebinding.FeatureSessionExport); err != nil {
		return err
	}
	format, filename, err := parseExportArgument(argument)
	if err != nil {
		return err
	}
	sessionID, workspace := a.session.current.ID, a.session.current.Workspace.Path
	title := a.session.current.Title
	started := a.runApplicationOperation(sessionOutputOperation, false,
		func(ctx context.Context) (sessionOutputResult, error) {
			document, err := a.transfers.ExportSession(ctx, session.ExportRequest{SessionID: sessionID, Format: format})
			if err != nil {
				return sessionOutputResult{}, err
			}
			path, err := a.artifacts.Publish(workspace, title, filename, document)
			return sessionOutputResult{sessionID: sessionID, value: path}, err
		},
		func(result sessionOutputResult, err error) {
			if err != nil {
				a.message("export session failed: " + err.Error())
				return
			}
			a.message("exported session · " + result.value)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func parseExportArgument(argument string) (session.DocumentFormat, string, error) {
	argument = strings.TrimSpace(argument)
	formatName, filename, found := strings.Cut(argument, " ")
	if !found {
		formatName, filename = argument, ""
	}
	format, err := session.ParseDocumentFormat(formatName)
	if err != nil {
		return "", "", err
	}
	return format, strings.TrimSpace(filename), nil
}
