package fileobservation

import (
	"errors"
	"os"
	"runtime"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

const shutdownHint = 1

// Queued events retain this pinned registration identity across descriptor reuse.
type pathBinding struct {
	fd       int
	identity *byte
	pin      runtime.Pinner
}

func (b *pathBinding) close() error {
	err := unix.Close(b.fd)
	b.pin.Unpin()
	return err
}

// fsnotify's kqueue backend infers directory changes from child names, losing
// same-name symlink replacements. The content scanner supplies exact admitted
// files and parents; raw vnode hints wake its single fingerprint baseline.
type pathHints struct {
	mu        sync.Mutex
	queue     int
	paths     map[string]*pathBinding
	events    chan fsnotify.Event
	errors    chan error
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func newPathObservation() (pathObservation, error) {
	queue, err := unix.Kqueue()
	if err != nil {
		return pathObservation{}, err
	}
	unix.CloseOnExec(queue)
	_, err = unix.Kevent(queue, []unix.Kevent_t{{Ident: shutdownHint, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD | unix.EV_CLEAR}}, nil, nil)
	if err != nil {
		return pathObservation{}, errors.Join(err, unix.Close(queue))
	}
	hints := &pathHints{
		queue: queue, paths: make(map[string]*pathBinding),
		events: make(chan fsnotify.Event), errors: make(chan error),
		done: make(chan struct{}), exited: make(chan struct{}),
	}
	go hints.run()
	return pathObservation{registry: hints, events: hints.events, errors: hints.errors}, nil
}

func (d *pathHints) Add(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	select {
	case <-d.done:
		return fsnotify.ErrClosed
	default:
	}
	if _, present := d.paths[path]; present {
		return nil
	}
	fd, err := unix.Open(path, unix.O_EVTONLY|unix.O_CLOEXEC|unix.O_SYMLINK, 0)
	if err != nil {
		return &os.PathError{Op: "watch", Path: path, Err: err}
	}
	binding := &pathBinding{fd: fd, identity: new(byte)}
	binding.pin.Pin(binding.identity)
	event := unix.Kevent_t{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_ATTRIB | unix.NOTE_REVOKE, Udata: binding.identity}
	if _, err := unix.Kevent(d.queue, []unix.Kevent_t{event}, nil, nil); err != nil {
		return errors.Join(err, binding.close())
	}
	d.paths[path] = binding
	return nil
}

// Vnode hints need the entry's inode; Add binds symlinks without following them.
func metadataWatchPath(path string, _ os.FileMode) string { return path }

func (d *pathHints) Remove(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	binding, present := d.paths[path]
	if !present {
		return fsnotify.ErrNonExistentWatch
	}
	delete(d.paths, path)
	return binding.close()
}

func (d *pathHints) WatchList() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	paths := make([]string, 0, len(d.paths))
	for path := range d.paths {
		paths = append(paths, path)
	}
	return paths
}

func (d *pathHints) run() {
	defer close(d.exited)
	defer close(d.events)
	defer close(d.errors)
	events := make([]unix.Kevent_t, 16)
	for {
		count, err := unix.Kevent(d.queue, nil, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			d.report(err)
			return
		}
		for _, event := range events[:count] {
			if !d.handle(event) {
				return
			}
		}
	}
}

// handle forwards one kernel event as a change hint, retiring the watch of a
// path that was deleted, renamed or revoked. It reports whether to continue.
func (d *pathHints) handle(event unix.Kevent_t) bool {
	select {
	case <-d.done:
		return false
	default:
	}
	if event.Flags&unix.EV_ERROR != 0 && event.Data != 0 {
		d.report(unix.Errno(event.Data))
		return false
	}
	if event.Filter == unix.EVFILT_USER {
		return true
	}
	if event.Fflags&(unix.NOTE_DELETE|unix.NOTE_RENAME|unix.NOTE_REVOKE) != 0 {
		if err := d.forget(event.Udata); err != nil && !d.report(err) {
			return false
		}
	}
	select {
	case d.events <- fsnotify.Event{Op: fsnotify.Write}:
		return true
	case <-d.done:
		return false
	}
}

// forget closes the watch whose kernel identity the event names.
func (d *pathHints) forget(identity *byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for path, binding := range d.paths {
		if binding.identity == identity {
			delete(d.paths, path)
			return binding.close()
		}
	}
	return nil
}

// report delivers an error unless the watcher is closing; it reports whether
// the error was delivered.
func (d *pathHints) report(err error) bool {
	select {
	case d.errors <- err:
		return true
	case <-d.done:
		return false
	}
}

// wake interrupts a blocked Kevent so run observes shutdown.
func (d *pathHints) wake() error {
	for {
		_, err := unix.Kevent(d.queue, []unix.Kevent_t{{Ident: shutdownHint, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}}, nil, nil)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func (d *pathHints) Close() error {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		close(d.done)
		wakeErr := d.wake()
		d.mu.Unlock()
		<-d.exited
		d.mu.Lock()
		defer d.mu.Unlock()
		var errs []error
		errs = append(errs, wakeErr)
		for path, binding := range d.paths {
			errs = append(errs, binding.close())
			delete(d.paths, path)
		}
		d.closeErr = errors.Join(append(errs, unix.Close(d.queue))...)
	})
	return d.closeErr
}
