package terminal

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestApplicationClocksPreserveCancellationAndCapacityRefusal(t *testing.T) {
	for name, schedule := range map[string]func(*app, time.Duration, func()) func(){
		"after": (*app).after,
		"every": (*app).every,
	} {
		for _, lifetime := range []string{"external cancellation", "closed scope", "live capacity refusal"} {
			t.Run(name+"/"+lifetime, func(t *testing.T) {
				host := uitest.New(t, uitest.Config{Width: 20, Height: 4})
				var active *app
				var applied bool
				screen, err := program.NewScreen(program.ScreenConfig{
					Config: program.Config{Host: host, Resources: 1},
					Build: func(loop *program.Runtime) (scene.Element, error) {
						defer loop.Quit()
						ctx, cancel := context.WithCancel(t.Context())
						defer cancel()
						scope, err := loop.NewScope(program.ScopeConfig{Context: ctx})
						if err != nil {
							return scene.Element{}, err
						}
						defer scope.Close()
						active = &app{ctx: ctx, loop: loop, scope: scope}
						switch lifetime {
						case "external cancellation":
							cancel()
						case "closed scope":
							scope.Close()
						}
						if stop := schedule(active, time.Hour, func() { applied = true }); stop != nil {
							stop()
							return scene.Element{}, fmt.Errorf("clock was admitted after %s", lifetime)
						}
						return passiveElement(kit.Label{Text: "clock admission"}), nil
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := screen.Run(t.Context()); err != nil {
					t.Fatal(err)
				}
				var want error
				if lifetime == "live capacity refusal" {
					want = program.ErrCapacity
				}
				if !errors.Is(active.runtimeErr, want) {
					t.Fatalf("clock admission failure = %v, want %v", active.runtimeErr, want)
				}
				if applied {
					t.Fatal("refused clock applied its callback")
				}
			})
		}
	}
}
