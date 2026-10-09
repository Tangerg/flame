package procgroup

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	processExiting = 0x00002000 // P_WEXIT from Darwin's public sys/proc.h.
	processZombie  = 5          // SZOMB from Darwin's public sys/proc.h.
)

func stopGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	if !errors.Is(err, syscall.EPERM) {
		return err
	}
	// XNU may report EPERM when every remaining member is already exiting.
	// Only kernel termination evidence can classify that as a finished group.
	members, inspectErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	if inspectErr != nil {
		return errors.Join(err, fmt.Errorf("inspect group membership: %w", inspectErr))
	}
	if groupHasLiveProcesses(members) {
		return err
	}
	return os.ErrProcessDone
}

func groupHasLiveProcesses(members []unix.KinfoProc) bool {
	return slices.ContainsFunc(members, func(member unix.KinfoProc) bool {
		return member.Proc.P_stat != processZombie && member.Proc.P_flag&processExiting == 0
	})
}
