package main

import (
	"cmp"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
)

const defaultAgent = "opencode"

var profileName = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,63}$")

type agentProfile struct {
	wire.Agent
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	NativeName string   `json:"nativeName,omitempty"`
}

type agentProgram struct {
	agentProfile
	path      string
	authority string
}

func readAgents(openCode, configured string) (map[string]agentProfile, error) {
	defaultProfile := agentProfile{
		Agent: wire.Agent{ID: defaultAgent, Name: "OpenCode"}, Executable: openCode,
		Args: []string{"acp"}, NativeName: defaultAgent,
	}
	profiles := map[string]agentProfile{defaultAgent: defaultProfile}
	if configured == "" {
		return profiles, nil
	}
	var declared []agentProfile
	if err := json.Unmarshal([]byte(configured), &declared, json.RejectUnknownMembers(true)); err != nil {
		return nil, errors.New("acp: agent profiles must be a JSON array")
	}
	for _, item := range declared {
		if _, exists := profiles[item.ID]; exists || !profileName.MatchString(item.ID) {
			return nil, errors.New("acp: agent profiles require unique IDs")
		}
		if !filepath.IsAbs(item.Executable) || len(item.Executable) > 4096 ||
			strings.ContainsFunc(item.Executable, unicode.IsControl) {
			return nil, errors.New("acp: agent profiles require an absolute executable")
		}
		if item.Name == "" {
			item.Name = item.ID
		}
		if !validAgentText(item.Name) || item.NativeName != "" && !validAgentText(item.NativeName) {
			return nil, errors.New("acp: invalid agent profile name")
		}
		if item.Registry != "" || item.Version != "" {
			if !profileName.MatchString(item.Registry) || !validAgentText(item.Version) || item.NativeName == "" {
				return nil, errors.New("acp: a registry profile requires a pinned version and native name")
			}
		}
		for _, arg := range item.Args {
			if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
				return nil, errors.New("acp: invalid agent argument")
			}
		}
		profiles[item.ID] = item
	}
	return profiles, nil
}

func validAgentText(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 128 &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func agentID(id string) string {
	if id == "" {
		return defaultAgent
	}
	return id
}

func (c *connector) agents() wire.AgentsResponse {
	items := make([]wire.Agent, 0, len(c.profiles))
	for _, profile := range c.profiles {
		items = append(items, profile.Agent)
	}
	slices.SortFunc(items, func(a, b wire.Agent) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return wire.AgentsResponse{Agents: items}
}

func (c *connector) program(target agentTarget) (agentProgram, error) {
	profile, known := c.profiles[target.agent]
	if !known {
		return agentProgram{}, refuse("the ACP agent profile is unavailable")
	}
	c.mu.Lock()
	link := c.links[target]
	c.mu.Unlock()
	if link != nil && !link.closed.Load() {
		return link.program, nil
	}
	path := profile.Executable
	if path == "" && profile.ID == defaultAgent {
		var err error
		path, err = exec.LookPath(defaultAgent)
		if err != nil {
			return agentProgram{}, refuse("configure the absolute path to the OpenCode executable")
		}
	}
	if !filepath.IsAbs(path) {
		return agentProgram{}, refuse("the ACP executable must be an absolute path")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return agentProgram{}, refuse("the configured ACP executable is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return agentProgram{}, refuse("the configured ACP executable is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	self, selfErr := os.Executable()
	own, ownErr := os.Stat(self)
	if err != nil || !info.Mode().IsRegular() || selfErr != nil || ownErr != nil || os.SameFile(info, own) {
		return agentProgram{}, refuse("the configured ACP executable is unavailable")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return agentProgram{}, fmt.Errorf("acp: fingerprint the configured program: %w", err)
	}
	key, err := json.Marshal(struct {
		Profile agentProfile
		Path    string
		Program string
	}{profile, path, fmt.Sprintf("%x", digest.Sum(nil))})
	if err != nil {
		return agentProgram{}, err
	}
	return agentProgram{agentProfile: profile, path: path, authority: fmt.Sprintf("%x", sha256.Sum256(key))}, nil
}
