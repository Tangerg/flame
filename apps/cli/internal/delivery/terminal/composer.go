package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/kit"
)

const fileElement base.ElementKind = 1

const draftPersistenceDelay = 150 * time.Millisecond

type composerContent struct {
	*kit.Editor
	chips map[base.ElementID]authoring.Attachment
}

func (c *composerContent) Controller() *base.Editor {
	if c.Editor == nil {
		c.Editor = kit.NewEditor(kit.EditorConfig{})
	}
	return c.Editor.Controller()
}

type promptComposer struct {
	composerContent
	observe        func()
	drafts         draftAutosave
	recall         historyCursor
	attachments    *attachment.Resolver
	workbench      *workbench.Store
	editor         *draftEditor
	terminal       program.Session
	localDirectory string
	session        *sessionState
	appendNote     func(label, body string)

	ctx      context.Context
	after    func(time.Duration, func()) func()
	closed   func() bool
	notify   func(string)
	report   func(workbenchConcern, error)
	validate func(authoring.Message) error
	cleared  func()
	busy     func() bool
}

type attachmentInsertion struct {
	item  authoring.Attachment
	count int
}

type historyCursor struct {
	entries  []authoring.Message
	offset   int
	original authoring.Message
}

func (h *historyCursor) reset() {
	*h = historyCursor{}
}

func (h *historyCursor) previous(store *workbench.Store, current authoring.Message) (authoring.Message, bool) {
	if h.offset == 0 {
		h.entries = store.History()
	}
	if h.offset == len(h.entries) {
		return authoring.Message{}, false
	}
	if h.offset == 0 {
		h.original = current.Clone()
	}
	h.offset++
	return h.entries[len(h.entries)-h.offset].Clone(), true
}

func (h *historyCursor) next() (authoring.Message, bool) {
	if h.offset == 0 {
		return authoring.Message{}, false
	}
	h.offset--
	if h.offset == 0 {
		draft := h.original
		h.reset()
		return draft, true
	}
	return h.entries[len(h.entries)-h.offset].Clone(), true
}

func (c *promptComposer) resolveAttachment(path string) (attachmentInsertion, error) {
	if c.attachments == nil {
		return attachmentInsertion{}, errors.New("attachments are unavailable in this session")
	}
	current, err := c.message()
	if err != nil {
		return attachmentInsertion{}, err
	}
	item, err := c.attachments.Resolve(c.ctx, path)
	if err != nil {
		return attachmentInsertion{}, err
	}
	if err := c.validate(authoring.Message{Attachments: []authoring.Attachment{item}}); err != nil {
		return attachmentInsertion{}, err
	}
	for _, attached := range current.Attachments {
		if attached.Path == item.Path {
			return attachmentInsertion{}, fmt.Errorf("%s is already attached", item.Name)
		}
	}
	if len(current.Attachments) >= authoring.MaxMessageAttachments {
		return attachmentInsertion{}, fmt.Errorf("a prompt accepts at most %d attachments", authoring.MaxMessageAttachments)
	}
	return attachmentInsertion{item: item, count: len(current.Attachments) + 1}, nil
}

func (c *promptComposer) insertAttachment(insertion attachmentInsertion) {
	item := insertion.item
	c.attach(item)
	c.scheduleDraftPersistence()
	c.notify(fmt.Sprintf("attached %s · %s · %d/%d", item.Name, item.MimeType, insertion.count, authoring.MaxMessageAttachments))
}

func (c *promptComposer) addAttachment(path string) error {
	insertion, err := c.resolveAttachment(path)
	if err != nil {
		return err
	}
	c.insertAttachment(insertion)
	return nil
}

func (c *promptCompletion) acceptAttachmentCompletion(path string, token base.Token) {
	insertion, err := c.composer.resolveAttachment(path)
	if err != nil {
		c.notify(err.Error())
		return
	}
	c.gate.Reset()
	c.composer.Controller().Replace(token.Line, token.Start-len(token.Trigger.Prefix), token.End, "")
	c.composer.insertAttachment(insertion)
}

func (c *promptComposer) removeAttachment(argument string) error {
	name, err := c.detach(argument)
	if err != nil {
		return err
	}
	c.scheduleDraftPersistence()
	if name == "" {
		c.notify("removed all attachments")
		return nil
	}
	c.notify("detached " + name)
	return nil
}

func (c *composerContent) attach(item authoring.Attachment) {
	element := c.Controller().InsertElement(fileElement, "@"+item.Name)
	if c.chips == nil {
		c.chips = make(map[base.ElementID]authoring.Attachment)
	}
	c.chips[element.ID] = item
}

func (c *composerContent) insertAttachments(items []authoring.Attachment) {
	for _, item := range items {
		c.attach(item)
	}
}

func (c *composerContent) detach(argument string) (string, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return "", errors.New("/detach needs a file name, number, or all")
	}
	elements := c.Controller().Elements()
	if len(elements) == 0 {
		return "", errors.New("the composer has no attachments")
	}
	if isAllArgument(argument) {
		for _, element := range elements {
			if element.Kind == fileElement {
				c.Controller().RemoveElement(element.ID)
			}
		}
		return "", nil
	}
	element, item, err := c.find(elements, argument)
	if err != nil {
		return "", err
	}
	c.Controller().RemoveElement(element.ID)
	return item.Name, nil
}

type composerAttachment struct {
	element base.Element
	item    authoring.Attachment
}

func (c *composerContent) find(elements []base.Element, argument string) (base.Element, authoring.Attachment, error) {
	attached := make([]composerAttachment, 0, len(elements))
	for _, element := range elements {
		item, ok := c.chips[element.ID]
		if !ok || element.Kind != fileElement {
			continue
		}
		attached = append(attached, composerAttachment{element: element, item: item})
	}
	if position, err := strconv.Atoi(argument); err == nil {
		if position > 0 && position <= len(attached) {
			match := attached[position-1]
			return match.element, match.item, nil
		}
		return base.Element{}, authoring.Attachment{}, fmt.Errorf("attachment %q is not in the composer", argument)
	}
	if matches := matchingAttachments(attached, func(item authoring.Attachment) bool { return argument == item.Name }); len(matches) > 0 {
		return uniqueAttachment(argument, matches)
	}
	if matches := matchingAttachments(attached, func(item authoring.Attachment) bool { return argument == filepathBase(item.Name) }); len(matches) > 0 {
		return uniqueAttachment(argument, matches)
	}
	return base.Element{}, authoring.Attachment{}, fmt.Errorf("attachment %q is not in the composer", argument)
}

func matchingAttachments(attached []composerAttachment, matches func(authoring.Attachment) bool) []composerAttachment {
	selected := make([]composerAttachment, 0, len(attached))
	for _, candidate := range attached {
		if matches(candidate.item) {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func uniqueAttachment(argument string, matches []composerAttachment) (base.Element, authoring.Attachment, error) {
	if len(matches) != 1 {
		return base.Element{}, authoring.Attachment{}, fmt.Errorf("attachment %q is ambiguous; use its number or full name", argument)
	}
	return matches[0].element, matches[0].item, nil
}

func filepathBase(path string) string {
	if at := strings.LastIndexAny(path, "/\\"); at >= 0 {
		return path[at+1:]
	}
	return path
}

func (c *promptComposer) showAttachments() {
	message, err := c.message()
	if err != nil {
		c.notify(err.Error())
		return
	}
	if len(message.Attachments) == 0 {
		c.notify("the composer has no attachments")
		return
	}
	lines := make([]string, 0, len(message.Attachments))
	for i, item := range message.Attachments {
		lines = append(lines, fmt.Sprintf("%d. %s · %s · %d bytes", i+1, item.Name, item.MimeType, item.Size))
	}
	c.appendNote("attachments", strings.Join(lines, "\n"))
}

func (c *composerContent) message() (authoring.Message, error) {
	editor := c.Controller()
	lines := strings.Split(editor.Text(), "\n")
	elements := editor.Elements()
	attachments, err := c.attachments(elements)
	if err != nil {
		return authoring.Message{}, err
	}
	if err := stripAttachmentElements(lines, elements); err != nil {
		return authoring.Message{}, err
	}
	return authoring.Message{Text: strings.Join(lines, "\n"), Attachments: attachments}, nil
}

func (c *composerContent) attachments(elements []base.Element) ([]authoring.Attachment, error) {
	attachments := make([]authoring.Attachment, 0, len(elements))
	for _, element := range elements {
		if element.Kind != fileElement {
			continue
		}
		item, ok := c.chips[element.ID]
		if !ok {
			return nil, fmt.Errorf("attachment chip %q lost its backing value", element.Text())
		}
		attachments = append(attachments, item)
	}
	return attachments, nil
}

func stripAttachmentElements(lines []string, elements []base.Element) error {
	for _, element := range slices.Backward(elements) {
		if element.Kind != fileElement || element.Line < 0 || element.Line >= len(lines) {
			continue
		}
		line := lines[element.Line]
		if element.Start < 0 || element.End > len(line) || element.Start >= element.End {
			return errors.New("attachment chip has an invalid editor range")
		}
		end := element.End
		if end < len(line) && line[end] == ' ' {
			end++
		}
		lines[element.Line] = line[:element.Start] + line[end:]
	}
	return nil
}

func (c *promptComposer) clearComposer() {
	c.clear()
	c.cleared()
}

// Undo history is dropped so undo cannot bring back a chip whose attachment was
// released.
func (c *composerContent) clear() {
	c.Controller().Clear()
	c.Controller().ForgetHistory()
	clear(c.chips)
}

func (c *composerContent) draft() (authoring.Message, bool, error) {
	editor := c.Controller()
	if editor.Empty() && len(editor.Elements()) == 0 {
		return authoring.Message{}, false, nil
	}
	message, err := c.message()
	if err != nil {
		return authoring.Message{}, false, err
	}
	return message, true, nil
}

func (c *promptComposer) persistDraft() error {
	c.drafts.cancel()
	message, _, err := c.draft()
	if err == nil {
		err = c.saveDraft(message)
	}
	c.report(workbenchDraft, err)
	return err
}

type draftAutosave struct {
	writer  *draftPersistence
	pending func()
}

func (d *draftAutosave) schedule(after func(time.Duration, func()) func(), capture func() (string, authoring.Message, error), report func(error)) {
	d.cancel()
	d.pending = after(draftPersistenceDelay, func() {
		d.pending = nil
		sessionID, message, err := capture()
		if err != nil {
			report(err)
			return
		}
		if err := d.writer.Schedule(sessionID, message); err != nil {
			report(err)
		}
	})
}

func (d *draftAutosave) cancel() {
	if d.pending == nil {
		return
	}
	d.pending()
	d.pending = nil
}

func (c *promptComposer) scheduleDraftPersistence() {
	if c.closed() {
		return
	}
	c.drafts.schedule(c.after, func() (string, authoring.Message, error) {
		message, _, err := c.draft()
		return c.session.current.ID, message, err
	}, func(err error) { c.report(workbenchDraft, err) })
}

func (c *promptComposer) saveDraft(message authoring.Message) error {
	sessionID := c.session.current.ID
	return c.commitAuthoring(func() error {
		if sessionID == "" {
			return nil
		}
		return c.workbench.SaveDraft(sessionID, message)
	})
}

func (c *promptComposer) commitAuthoring(apply func() error) error {
	err := c.drafts.writer.Commit(apply)
	if err == nil {
		c.drafts.cancel()
	}
	return err
}

func (c *promptComposer) commitDraft(message authoring.Message) error {
	err := c.saveDraft(message)
	c.report(workbenchDraft, err)
	if err != nil {
		return err
	}
	c.replaceDraft(message)
	return nil
}

func (c *promptComposer) recoverDraft(message authoring.Message) error {
	c.replaceDraft(message)
	return c.persistDraft()
}

func (c *promptComposer) replaceDraft(message authoring.Message) {
	c.recall.reset()
	c.restoreComposer(message)
}

func (c *promptComposer) restoreComposer(message authoring.Message) {
	c.clearComposer()
	c.insertAttachments(message.Attachments)
	if message.Text != "" {
		c.Controller().Insert(message.Text)
	}
	c.scheduleDraftPersistence()
}

func (c *promptComposer) recallPrevious() bool {
	current, err := c.message()
	if err != nil {
		c.notify(err.Error())
		return true
	}
	message, ok := c.recall.previous(c.workbench, current)
	if ok {
		c.restoreComposer(message)
	}
	return ok
}

func (c *promptComposer) recallNext() bool {
	message, ok := c.recall.next()
	if ok {
		c.restoreComposer(message)
	}
	return ok
}
