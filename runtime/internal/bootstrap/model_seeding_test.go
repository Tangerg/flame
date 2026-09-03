package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestSeedUtilityRoleOnlySeedsAnAbsentDurableChoice(t *testing.T) {
	tests := []struct {
		name         string
		stored       modelref.Role
		present      bool
		configured   config.Settings
		wantSaves    int
		wantProvider string
		wantModel    string
	}{
		{
			name: "absent role uses config", configured: config.Settings{
				Provider: "deepseek", Model: "deepseek-chat", UtilityModel: "deepseek-reasoner",
			},
			wantSaves: 1, wantProvider: "deepseek", wantModel: "deepseek-reasoner",
		},
		{
			name: "configured role wins", stored: mustBootstrapRole(t, "openai", "gpt-5-mini"), present: true,
			configured: config.Settings{
				Provider: "deepseek", Model: "deepseek-chat", UtilityModel: "deepseek-reasoner",
			},
			wantProvider: "openai", wantModel: "gpt-5-mini",
		},
		{
			name: "explicit inheritance wins", present: true,
			configured: config.Settings{
				Provider: "deepseek", Model: "deepseek-chat", UtilityModel: "deepseek-reasoner",
			},
		},
		{
			name: "main model needs no seed", configured: config.Settings{
				Provider: "deepseek", Model: "deepseek-chat", UtilityModel: "deepseek-chat",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &utilityRoleSeedStore{role: test.stored, present: test.present}
			if err := SeedUtilityRole(t.Context(), store, test.configured); err != nil {
				t.Fatal(err)
			}
			if store.saves != test.wantSaves || store.role.Provider() != test.wantProvider || store.role.Model() != test.wantModel {
				t.Fatalf("seeded role = (%q, %q), saves=%d", store.role.Provider(), store.role.Model(), store.saves)
			}
		})
	}
}

type utilityRoleSeedStore struct {
	role    modelref.Role
	present bool
	saves   int
}

func (s *utilityRoleSeedStore) LoadUtilityRole(context.Context) (modelref.Role, bool, error) {
	return s.role, s.present, nil
}

func (s *utilityRoleSeedStore) SaveUtilityRole(_ context.Context, role modelref.Role) error {
	s.role = role
	s.present = true
	s.saves++
	return nil
}
