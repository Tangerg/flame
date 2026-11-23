package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/frame/process"
	"github.com/samber/lo"
)

type modelProviderAdapters interface {
	Profiles(context.Context) ([]modelprovider.Profile, error)
	Profile(context.Context, string) (modelprovider.Profile, error)
	RemoteModels(context.Context, modelprovider.Connection) ([]string, error)
	Verify(context.Context, modelprovider.Connection) error
	OpenChat(context.Context, flamemodel.Selection, modelprovider.Connection) (modeladapter.ResolvedChat, error)
	RestoreChat(context.Context, flamemodel.Selection, *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error)
	OpenEmbedding(context.Context, flamemodel.Selection, modelprovider.Connection) (modeladapter.ResolvedEmbedding, error)
}

type assemblyConfig struct {
	stores              *persistence.Bundle
	processRunner       process.Runner
	firstPartyDirectory string

	buildID string

	modelProviders modelProviderAdapters

	skillsUserDir string

	maintenance executionadapter.RunMaintenance

	resources []terminalResource

	sandboxShell         bool
	sandboxReadOnlyPaths []string

	sandboxDir string

	providerRegistry models.ProviderRegistry

	approvalMode approval.Mode

	defaultSelection flamemodel.Selection

	checkpointDir string

	userHome string

	defaultWorkspacePath string

	toolResultOffloadThreshold *int
}

func validateAssemblyConfig(c assemblyConfig) error {
	if c.stores == nil {
		return errors.New("runtime: Stores is required")
	}
	if lo.IsNil(c.processRunner) {
		return errors.New("runtime: process runner is required")
	}
	if err := c.validatePaths(); err != nil {
		return err
	}
	if c.modelProviders != nil && lo.IsNil(c.modelProviders) {
		return errors.New("runtime: model providers are typed nil")
	}
	if _, err := runs.ParseBuild(c.buildID); err != nil {
		return fmt.Errorf("runtime: BuildID: %w", err)
	}
	if lo.IsNil(c.providerRegistry) {
		return errors.New("runtime: ProviderRegistry is required")
	}
	if err := c.defaultSelection.ValidateExact(); err != nil {
		return fmt.Errorf("runtime: default model selection: %w", err)
	}
	return nil
}

func (c assemblyConfig) validatePaths() error {
	type configuredPath struct{ name, value string }
	for _, required := range []configuredPath{
		{name: "UserHome", value: c.userHome},
		{name: "DefaultWorkspacePath", value: c.defaultWorkspacePath},
		{name: "SkillsUserDir", value: c.skillsUserDir},
	} {
		if required.value == "" {
			return fmt.Errorf("runtime: %s is required", required.name)
		}
		if !filepath.IsAbs(required.value) {
			return fmt.Errorf("runtime: %s must be absolute", required.name)
		}
	}
	for _, optional := range []configuredPath{
		{name: "SandboxDir", value: c.sandboxDir},
		{name: "CheckpointDir", value: c.checkpointDir},
	} {
		if optional.value != "" && !filepath.IsAbs(optional.value) {
			return fmt.Errorf("runtime: %s must be absolute when set", optional.name)
		}
	}
	for index, configuredPath := range c.sandboxReadOnlyPaths {
		if configuredPath != "" && !filepath.IsAbs(configuredPath) {
			return fmt.Errorf("runtime: SandboxReadOnlyPaths[%d] must be absolute when set", index)
		}
	}
	return nil
}
