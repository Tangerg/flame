package main

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	skillsrc "github.com/Tangerg/scope/skills"
	skillstool "github.com/Tangerg/scope/tools/skills"
)

type skillSampleSource []skillsrc.Summary

func (s skillSampleSource) List(ctx context.Context) ([]skillsrc.Summary, error) {
	return slices.Clone(s), ctx.Err()
}

func (skillSampleSource) Load(context.Context, string) (*skillsrc.Skill, error) {
	return nil, fs.ErrNotExist
}

func (skillSampleSource) OpenResource(context.Context, string, string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

// The current Scope producer owns both page encoding and continuation identity.
func writeSkillPageSamples(path string) error {
	source := skillSampleSource{
		{Name: "review-diff", Description: "Keep &amp; and <img src=x onerror=alert(1)> literal."},
		{Name: "trace-flaky-test", Description: "Find the shared state.\nKeep the second line."},
		{Name: "verify-release", Description: strings.Repeat("Keep release evidence. ", 8) + "Complete."},
	}
	empty, err := skillListSample(skillSampleSource{}, 0, nil)
	if err != nil {
		return err
	}
	complete, err := skillListSample(source, 0, nil)
	if err != nil {
		return err
	}
	first, err := skillListSample(source, 400, nil)
	if err != nil {
		return err
	}
	var page skillstool.ListPage
	if err := json.Unmarshal([]byte(first), &page); err != nil {
		return err
	}
	if len(page.Skills) != 2 || page.NextCursor == nil {
		return fmt.Errorf("skill sample must exercise a two-summary page and continuation")
	}
	last, err := skillListSample(source, 400, page.NextCursor)
	if err != nil {
		return err
	}
	page = skillstool.ListPage{}
	if err := json.Unmarshal([]byte(last), &page); err != nil {
		return err
	}
	if len(page.Skills) != 1 || page.NextCursor != nil {
		return fmt.Errorf("skill sample continuation must complete the listing")
	}
	return writeJSON(path, struct {
		Empty    string `json:"empty"`
		Complete string `json:"complete"`
		First    string `json:"first"`
		Last     string `json:"last"`
	}{empty, complete, first, last})
}

func skillListSample(source skillSampleSource, budget int64, cursor *skillstool.Cursor) (string, error) {
	tools, err := skillstool.NewTools(source, skillstool.Config{MaxOutputBytes: budget})
	if err != nil {
		return "", err
	}
	arguments, err := json.Marshal(skillstool.ListSkillsRequest{Cursor: cursor})
	if err != nil {
		return "", err
	}
	for _, executable := range tools {
		binding, err := toolcontract.Bind(executable)
		if err != nil {
			return "", err
		}
		if binding.Contract().Definition().Name != "list_skills" {
			continue
		}
		invocation, err := binding.Contract().Prepare(chat.ToolCall{
			ID: "skill_page_sample", Name: "list_skills", Arguments: string(arguments),
		}, nil)
		if err != nil {
			return "", err
		}
		result, err := binding.Call(context.Background(), invocation)
		if err != nil {
			return "", err
		}
		text, ok := result.Output.Text()
		if !ok {
			return "", fmt.Errorf("skill sample result is not text")
		}
		return text, nil
	}
	return "", fmt.Errorf("skill sample producer has no list_skills tool")
}
