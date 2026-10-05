package runs

import (
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Loss names why recovery could not continue a Run. It is the one owner of
// the explanation a lost Run carries, so the reason the probe established is
// what the person sees instead of a generic restart message. The underlying
// error can carry paths or payload details and stays in the trace.
type Loss string

const (
	// LossRestart is a Run that was not parked at a waiting boundary.
	LossRestart Loss = "restart"
	// LossIsolatedWorkspace is a Run whose isolated scratch workspace does not
	// survive the executor that created it.
	LossIsolatedWorkspace Loss = "isolatedWorkspace"
	// LossWorkspaceUnavailable is a Run whose workspace cannot be restored.
	LossWorkspaceUnavailable Loss = "workspaceUnavailable"
	// LossOtherBuild is a waiting state written by a different Runtime build.
	LossOtherBuild Loss = "otherBuild"
	// LossWaitingStateUnavailable is waiting state that is missing, malformed
	// or not at a waiting boundary.
	LossWaitingStateUnavailable Loss = "waitingStateUnavailable"
	// LossConfigurationChanged is a Run whose Session or execution
	// configuration no longer matches the state it waited in.
	LossConfigurationChanged Loss = "configurationChanged"
)

func (l Loss) detail() (string, error) {
	switch l {
	case LossRestart:
		return "run lost on restart", nil
	case LossIsolatedWorkspace:
		return "run lost on restart: its isolated workspace does not survive a restart", nil
	case LossWorkspaceUnavailable:
		return "run lost on restart: its workspace is unavailable", nil
	case LossOtherBuild:
		return "run lost on restart: its waiting state was written by another Runtime build", nil
	case LossWaitingStateUnavailable:
		return "run lost on restart: its waiting state could not be restored", nil
	case LossConfigurationChanged:
		return "run lost on restart: its configuration changed while it was waiting", nil
	default:
		return "", fmt.Errorf("runs: unknown Run loss %q", l)
	}
}

func (l Loss) failure() (rundomain.Failure, error) {
	detail, err := l.detail()
	if err != nil {
		return rundomain.Failure{}, err
	}
	return rundomain.Failure{Kind: rundomain.FailureLost, Detail: detail}, nil
}

// WaitingResumption is the verdict of probing one waiting execution: it can
// resume, or it is lost for exactly one stated reason. Only ResumableWaiting
// grants resumption, so the zero value an erroring probe returns can never
// keep a tree alive.
type WaitingResumption struct {
	resumable bool
	loss      Loss
}

// ResumableWaiting reports that the waiting execution can resume.
func ResumableWaiting() WaitingResumption { return WaitingResumption{resumable: true} }

// UnresumableWaiting reports that the waiting execution is lost for loss.
func UnresumableWaiting(loss Loss) WaitingResumption {
	return WaitingResumption{loss: loss}
}

func (w WaitingResumption) Resumable() bool { return w.resumable }

// Loss is the reason an unresumable execution is lost.
func (w WaitingResumption) Loss() Loss { return w.loss }
