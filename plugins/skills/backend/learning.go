package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/qimu/sqlite"
	skillspec "github.com/Tangerg/scope/skills"
)

type learner struct {
	db          *sql.DB
	mu          sync.Mutex
	complexRuns map[string]int
	lastArchive time.Time
}

func openLearner(ctx context.Context, path string) (*learner, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema})
	if err != nil {
		return nil, err
	}
	return &learner{db: db, complexRuns: make(map[string]int)}, nil
}

type learningInputs interface {
	Conversation(context.Context) (skilllearning.Conversation, error)
	ReadSkill(context.Context, string) (*skilllearning.Skill, error)
	Complete(context.Context, string, string) (string, error)
}

func (l *learner) mine(ctx context.Context, run skilllearning.Run, inputs learningInputs) (skilllearning.MineResult, error) {
	if run.ToolCalls < 8 || !l.miningDue(run.SessionID) {
		return skilllearning.MineResult{}, nil
	}
	conversation, err := inputs.Conversation(ctx)
	if err != nil {
		return skilllearning.MineResult{}, err
	}
	if conversation.MessageCount < 4 {
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
	name, revision := strings.CutPrefix(strings.TrimSpace(first), "REVISE:")
	name = strings.TrimSpace(name)
	if revision {
		if name == "" {
			return skilllearning.MineResult{}, nil
		}
		current, err := inputs.ReadSkill(ctx, name)
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
		return skilllearning.MineResult{}, nil
	}
	if revision {
		skill.Name = name
	}
	return skilllearning.MineResult{Proposal: &skilllearning.Proposal{
		Skill: skilllearning.Skill{Name: skill.Name, Description: skill.Description, Instructions: skill.Instructions}, Revises: revision,
	}}, nil
}

func (l *learner) miningDue(session string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.complexRuns[session]++
	if l.complexRuns[session] < 3 {
		return false
	}
	// This counter bounds cost, not correctness. Attempts reset it even on
	// failure, and restarting the backend starts a fresh cadence.
	delete(l.complexRuns, session)
	return true
}

func (l *learner) archive(now time.Time) skilllearning.ArchiveAdvice {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.lastArchive.IsZero() && now.Sub(l.lastArchive) < 6*time.Hour {
		return skilllearning.ArchiveAdvice{}
	}
	l.lastArchive = now
	after := (30 * 24 * time.Hour).Milliseconds()
	return skilllearning.ArchiveAdvice{AfterMillis: &after}
}

func noSkill(document string) bool {
	return strings.EqualFold(strings.TrimSpace(document), "NO_SKILL")
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
