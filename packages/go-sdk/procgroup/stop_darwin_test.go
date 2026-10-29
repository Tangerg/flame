package procgroup

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestKernelTerminationEvidenceCannotHideLiveGroupMembers(t *testing.T) {
	sleeping := unix.KinfoProc{Proc: unix.ExternProc{P_stat: 2}}
	exiting := unix.KinfoProc{Proc: unix.ExternProc{P_stat: 2, P_flag: processExiting}}
	zombie := unix.KinfoProc{Proc: unix.ExternProc{P_stat: processZombie}}
	for _, test := range []struct {
		name    string
		members []unix.KinfoProc
		live    bool
	}{
		{name: "reaped"},
		{name: "exiting before zombie publication", members: []unix.KinfoProc{exiting}},
		{name: "awaiting reap", members: []unix.KinfoProc{zombie}},
		{name: "live descendant", members: []unix.KinfoProc{sleeping}, live: true},
		{name: "live among exiting", members: []unix.KinfoProc{exiting, sleeping, zombie}, live: true},
		{name: "unknown process state", members: []unix.KinfoProc{{}}, live: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := groupHasLiveProcesses(test.members); got != test.live {
				t.Fatalf("kernel group liveness = %v, want %v", got, test.live)
			}
		})
	}
}
