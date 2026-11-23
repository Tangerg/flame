package main

import (
	"context"
	"errors"
	"math"
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
	db          sqlite.Database
	mu          sync.Mutex
	complexRuns map[string]*miningSession
	visits      uint64
	lastArchive time.Time
}

func openLearner(ctx context.Context, path string) (*learner, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema, reviewsSchema})
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return &learner{db: db, complexRuns: make(map[string]*miningSession)}, nil
}

func (l *learner) mine(ctx context.Context, run skilllearning.Run, inputs remoteInputs) (skilllearning.MineResult, error) {
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
	name, revision := strings.CutPrefix(strings.TrimSpace(document), revisionMarker)
	name = strings.TrimSpace(name)
	var previous string
	if revision {
		var found bool
		document, previous, found, err = reviseSkill(ctx, inputs, name, conversation.Transcript)
		if err != nil || !found || noSkill(document) {
			return skilllearning.MineResult{}, err
		}
	}
	skill, err := skillspec.Parse([]byte(strings.TrimSpace(document)))
	if err != nil {
		return skilllearning.MineResult{}, errors.Join(errInvalidSkillDraft, err)
	}
	if revision {
		if err := skill.ValidateFor(name); err != nil {
			return skilllearning.MineResult{}, errors.Join(errInvalidSkillDraft, err)
		}
	}
	return skilllearning.MineResult{Proposal: &skilllearning.Proposal{
		Skill: skilllearning.Skill{Name: skill.Name, Description: skill.Description, Instructions: skill.Instructions}, Revises: revision, Previous: previous,
	}}, nil
}

func reviseSkill(ctx context.Context, inputs remoteInputs, name, transcript string) (document, previous string, found bool, err error) {
	if err := skillspec.ValidateName(name); err != nil {
		return "", "", false, errors.Join(errInvalidSkillDraft, err)
	}
	read, err := inputs.ReadSkill(ctx, name)
	if err != nil || read.Skill == nil {
		return "", "", false, err
	}
	current := read.Skill
	if current.Name != name {
		return "", "", false, errors.New("skill learning: source belongs to another skill")
	}
	input := "CURRENT SKILL.md\n---\nname: " + current.Name + "\ndescription: " + current.Description + "\n\n" + current.Instructions + "\n\nCONVERSATION\n---\n" + transcript
	document, err = inputs.Complete(ctx, skillRevisePrompt, input)
	return document, read.Revision, true, err
}

var errInvalidSkillDraft = errors.New("skill learning: model returned an invalid skill document")

type miningSession struct {
	runs      int
	lastVisit uint64
}

func (l *learner) miningDue(session string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.complexRuns[session]
	if entry == nil {
		if len(l.complexRuns) == maxTrackedSessions {
			l.forgetLeastRecentSession()
		}
		entry = &miningSession{}
		l.complexRuns[session] = entry
	}
	l.visits++
	entry.lastVisit = l.visits
	entry.runs++
	if entry.runs < runsPerMiningAttempt {
		return false
	}
	delete(l.complexRuns, session)
	return true
}

func (l *learner) forgetLeastRecentSession() {
	var oldest string
	oldestVisit := uint64(math.MaxUint64)
	for session, entry := range l.complexRuns {
		if entry.lastVisit < oldestVisit {
			oldest, oldestVisit = session, entry.lastVisit
		}
	}
	delete(l.complexRuns, oldest)
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
