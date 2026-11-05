package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

func openShellLog() (*os.File, string, error) {
	file, err := os.CreateTemp("", "flame-shell-*.log")
	if err != nil {
		return nil, "", err
	}
	// Unix keeps the open descriptor addressable after unlink. No command can
	// discover the log by pathname, and abrupt Runtime exit also reclaims it.
	// Windows retains the private 0600 file until the original owner closes it.
	if runtime.GOOS != "windows" {
		if err := os.Remove(file.Name()); err != nil {
			return nil, "", errors.Join(err, file.Close(), os.Remove(file.Name()))
		}
		return file, "", nil
	}
	return file, file.Name(), nil
}

// Write is the stdout/stderr sink of the original process. Capacity or disk
// failure cancels that process, preserving every successfully admitted byte and
// its error. It never discards an early diagnostic to make room for a later one.
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outputErr != nil {
		return 0, s.outputErr
	}
	if s.log == nil || s.logReleased {
		return 0, os.ErrClosed
	}
	requested := min(int64(len(p)), maxShellOutputBytes-s.logBytes)
	var allowed int64
	for {
		used := s.spoolBytes.Load()
		allowed = min(requested, maxRetainedOutputBytes-used)
		if s.spoolBytes.CompareAndSwap(used, used+allowed) {
			break
		}
	}
	n, err := s.log.WriteAt(p[:allowed], s.logBytes)
	s.logBytes += int64(n)
	s.spoolBytes.Add(int64(n) - allowed)
	if int64(n) < allowed && err == nil {
		err = io.ErrShortWrite
	}
	if allowed < int64(len(p)) && err == nil {
		err = ErrShellOutputLimit
	}
	if err != nil {
		s.outputErr = fmt.Errorf("exec: record shell output: %w", err)
		s.cancel()
	}
	return n, s.outputErr
}

func (s *Shell) read() (Output, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	output := Output{
		Dropped: s.outputErr != nil, Finished: s.finished(), Offset: s.readPos,
	}
	data := make([]byte, min(maxOutputPageBytes, s.logBytes-s.readPos))
	var readErr error
	if len(data) > 0 {
		if s.log == nil {
			readErr = os.ErrClosed
			data = nil
		} else {
			var n int
			n, readErr = s.log.ReadAt(data, s.readPos)
			data = data[:n]
		}
	}
	output.Text = string(data)
	output.NextOffset = s.readPos + int64(len(data))
	output.HasMore = output.NextOffset < s.logBytes
	var cleanupErr error
	if output.Finished {
		output.Info, output.ExitCode, output.Killed, output.Duration = s.exitInfo, s.exitCode, s.killed, s.duration
		cleanupErr = s.cleanup
	}
	if readErr != nil {
		return output, false, errors.Join(cleanupErr, s.outputErr, readErr)
	}
	s.readPos = output.NextOffset
	return output, output.Finished && !output.HasMore && cleanupErr == nil, errors.Join(cleanupErr, s.outputErr)
}

// releaseLog joins the existing process owner's final-read and teardown paths.
// A failed release retains the same handle and quota until its resource settles.
func (s *Shell) releaseLog() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logReleased {
		return nil
	}
	if s.log != nil {
		if err := s.log.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return err
		}
		s.log = nil
	}
	if s.logPath != "" {
		if err := os.Remove(s.logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		s.logPath = ""
	}
	s.spoolBytes.Add(-s.logBytes)
	s.logReleased = true
	return nil
}
