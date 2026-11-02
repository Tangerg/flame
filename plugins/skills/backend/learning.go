package main

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	skillspec "github.com/Tangerg/scope/skills"
)

const (
	minimumToolCalls            = 8
	runsPerMiningAttempt        = 3
	maxTrackedSessions          = 4096
	minimumConversationMessages = 4
	archiveCheckInterval        = 6 * time.Hour
	idleRetention               = 30 * 24 * time.Hour
)

type learner struct {
	db          *sql.DB
	mu          sync.Mutex
	complexRuns map[string]*list.Element
	recent      list.List
	lastArchive time.Time
}

func openLearner(ctx context.Context, path string) (*learner, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema, reviewsSchema})
	if err != nil {
		return nil, err
	}
	return &learner{db: db, complexRuns: make(map[string]*list.Element)}, nil
}

type learningInputs interface {
	Conversation(context.Context) (skilllearning.Conversation, error)
	ReadSkill(context.Context, string) (skilllearning.ReadResult, error)
	Complete(context.Context, string, string) (string, error)
}

func (l *learner) mine(ctx context.Context, run skilllearning.Run, inputs learningInputs) (skilllearning.MineResult, error) {
	if run.ToolCalls < minimumToolCalls || !l.miningDue(run.SessionID) {
		return skilllearning.MineResult{}, nil
	}
	conversation, err := inputs.Conversation(ctx)
	if err != nil {
		return skilllearning.MineResult{}, err
	}
	if conversation.MessageCount < minimumConversationMessages {
		return skilllearning.MineResult{}, nil
	}
	if len(conversation.Transcript) > skilllearning.MaxTranscriptBytes {
		return skilllearning.MineResult{}, errors.New("skill learning: conversation exceeds the transcript limit")
	}
	document, err := inputs.Complete(ctx, skillMinerPrompt, conversation.Transcript)
	if err != nil {
		return skilllearning.MineResult{}, err
	}
	if noSkill(document) {
		return skilllearning.MineResult{}, nil
	}
	first, _, _ := strings.Cut(document, "\n")
	name, revision := strings.CutPrefix(strings.TrimSpace(first), revisionMarker)
	name = strings.TrimSpace(name)
	var previous string
	if revision {
		if name == "" {
			return skilllearning.MineResult{}, nil
		}
		read, err := inputs.ReadSkill(ctx, name)
		current := read.Skill
		previous = read.Revision
		if err != nil {
			return skilllearning.MineResult{}, err
		}
		if current == nil {
			return skilllearning.MineResult{}, nil
		}
		if current.Name != name {
			return skilllearning.MineResult{}, errors.New("skill learning: source belongs to another skill")
		}
		input := "CURRENT SKILL.md\n---\nname: " + current.Name + "\ndescription: " + current.Description + "\n\n" + current.Instructions + "\n\nCONVERSATION\n---\n" + conversation.Transcript
		document, err = inputs.Complete(ctx, skillRevisePrompt, input)
		if err != nil {
			return skilllearning.MineResult{}, err
		}
		if noSkill(document) {
			return skilllearning.MineResult{}, nil
		}
	}
	skill, err := skillspec.Parse([]byte(unfence(document)))
	if err != nil {
		return skilllearning.MineResult{}, errInvalidSkillDraft
	}
	if revision {
		skill.Name = name
	}
	return skilllearning.MineResult{Proposal: &skilllearning.Proposal{
		Skill: skilllearning.Skill{Name: skill.Name, Description: skill.Description, Instructions: skill.Instructions}, Revises: revision, Previous: previous,
	}}, nil
}

var errInvalidSkillDraft = errors.New("skill learning: model returned an invalid skill document")

type miningSession struct {
	id   string
	runs int
}

func (l *learner) miningDue(session string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.complexRuns[session]
	if entry == nil {
		if l.recent.Len() == maxTrackedSessions {
			oldest := l.recent.Back()
			delete(l.complexRuns, oldest.Value.(*miningSession).id)
			l.recent.Remove(oldest)
		}
		entry = l.recent.PushFront(&miningSession{id: session})
		l.complexRuns[session] = entry
	}
	l.recent.MoveToFront(entry)
	count := entry.Value.(*miningSession)
	count.runs++
	if count.runs < runsPerMiningAttempt {
		return false
	}
	l.recent.Remove(entry)
	delete(l.complexRuns, session)
	return true
}

func (l *learner) archive(now time.Time) skilllearning.ArchiveAdvice {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.lastArchive.IsZero() && now.Sub(l.lastArchive) < archiveCheckInterval {
		return skilllearning.ArchiveAdvice{}
	}
	l.lastArchive = now
	return skilllearning.ArchiveAdvice{InactiveSince: new(now.Add(-idleRetention))}
}

func noSkill(document string) bool {
	return strings.EqualFold(strings.TrimSpace(document), declineSkill)
}

func unfence(document string) string {
	document = strings.TrimSpace(document)
	if strings.HasPrefix(document, "```") {
		if _, body, found := strings.Cut(document, "\n"); found {
			document = body
		}
		document = strings.TrimSuffix(strings.TrimRight(document, "\n"), "```")
	}
	return strings.TrimSpace(document)
}
