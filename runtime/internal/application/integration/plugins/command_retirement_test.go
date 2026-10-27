package plugins_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestPreparedCommandRetiresAcrossRestartWithoutLosingItsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	store := db.PluginCommands
	commands, err := plugins.NewCommands(store, db.Admission)
	if err != nil {
		t.Fatal(err)
	}
	provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", "installation"), ContributionID: "owner", Export: "operations"}
	resolve := func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }
	input := testsupport.JSONInput(t, `{"revision":2}`)
	if _, err := commands.Bind(t.Context(), "command", "review.update", input, resolve, capturedCommand); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = persistencefixture.Open(t, t.Context(), filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = db.PluginCommands
	commands, err = plugins.NewCommands(store, db.Admission)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := commands.Retire(t.Context(), "command"); err != nil {
			t.Fatal(err)
		}
	}
	held, err := store.PendingCommandDependencies(t.Context())
	if err != nil || len(held) != 0 {
		t.Fatalf("retired dependencies=%v, %v", held, err)
	}
	if _, err := commands.Get(t.Context(), "command"); !errors.Is(err, plugin.ErrCommandRetired) {
		t.Fatalf("recovery=%v", err)
	}
	if err := commands.Dispatch(t.Context(), "command", func(context.Context, plugin.CommandBinding) error {
		t.Fatal("retired command reached dispatch")
		return nil
	}); !errors.Is(err, plugin.ErrCommandRetired) {
		t.Fatalf("dispatch=%v", err)
	}
	if err := commands.Observe(t.Context(), "command"); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("retirement became result observation: %v", err)
	}
	retained, err := commands.Bind(t.Context(), "command", "review.update", input, func(context.Context) (plugins.CommandAdmission, error) {
		t.Fatal("retired identity resolved another provider")
		return plugins.CommandAdmission{}, nil
	}, capturedCommand)
	if err != nil || retained.Snapshot().Phase != plugin.CommandRetired {
		t.Fatalf("retained=%v, %v", retained, err)
	}
	if _, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{}`), resolve, capturedCommand); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("retired identity changed input: %v", err)
	}
	for _, phase := range []string{"prepared", "dispatched", "observed"} {
		if _, err := db.ExecContext(t.Context(), `UPDATE plugin_commands SET phase=? WHERE id='command'`, phase); err == nil {
			t.Fatalf("SQL moved retired command to %s", phase)
		}
	}
}

func TestCommandDispatchAndRetirementHaveExactlyOneWinner(t *testing.T) {
	for _, first := range []string{"dispatch", "retire"} {
		t.Run(first, func(t *testing.T) {
			db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := db.PluginCommands
			admission := db.Admission
			commands, err := plugins.NewCommands(store, admission)
			if err != nil {
				t.Fatal(err)
			}
			provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", "installation"), ContributionID: "owner", Export: "operations"}
			if _, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{}`), func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }, capturedCommand); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			blocked := &retirementStore{CommandStore: store, entered: entered, release: release}
			commands, err = plugins.NewCommands(blocked, admission)
			if err != nil {
				t.Fatal(err)
			}
			dispatch := func() error {
				return commands.Dispatch(t.Context(), "command", func(context.Context, plugin.CommandBinding) error { return nil })
			}
			retire := func() error { return commands.Retire(t.Context(), "command") }
			winner, loser := dispatch, retire
			if first == "retire" {
				winner, loser = retire, dispatch
			}
			won, lost := make(chan error, 1), make(chan error, 1)
			go func() { won <- winner() }()
			<-entered
			go func() { lost <- loser() }()
			close(release)
			if err := <-won; err != nil {
				t.Fatal(err)
			}
			want := plugin.ErrCommandUnconfirmed
			if first == "retire" {
				want = plugin.ErrCommandRetired
			}
			if err := <-lost; !errors.Is(err, want) {
				t.Fatalf("loser=%v, want %v", err, want)
			}
			held, err := store.PendingCommandDependencies(t.Context())
			if err != nil || (len(held) > 0) != (first == "dispatch") {
				t.Fatalf("dependencies=%v, %v", held, err)
			}
		})
	}
}

type retirementStore struct {
	plugins.CommandStore
	entered chan struct{}
	release chan struct{}
}

func (s *retirementStore) Save(ctx context.Context, command *plugin.Command) error {
	close(s.entered)
	<-s.release
	return s.CommandStore.Save(ctx, command)
}
