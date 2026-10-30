package fileobservation

import (
	"errors"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

const shutdownHint = 1

// fsnotify's kqueue backend infers directory changes from child names. Replacing
// a symlink under the same name can produce no event. Reconciliation needs the
// raw directory hint alongside file-write events. Both only wake the same
// content baseline, which owns all content and identity decisions.
type directoryHints struct {
	mu        sync.Mutex
	queue     int
	files     *fsnotify.Watcher
	paths     map[string]int
	events    chan fsnotify.Event
	errors    chan error
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func newDirectoryObservation() (directoryObservation, error) {
	files, err := fsnotify.NewWatcher()
	if err != nil {
		return directoryObservation{}, err
	}
	queue, err := unix.Kqueue()
	if err != nil {
		return directoryObservation{}, errors.Join(err, files.Close())
	}
	unix.CloseOnExec(queue)
	_, err = unix.Kevent(queue, []unix.Kevent_t{{Ident: shutdownHint, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD | unix.EV_CLEAR}}, nil, nil)
	if err != nil {
		return directoryObservation{}, errors.Join(err, unix.Close(queue), files.Close())
	}
	hints := &directoryHints{
		queue: queue, files: files, paths: make(map[string]int),
		events: make(chan fsnotify.Event), errors: make(chan error),
		done: make(chan struct{}), exited: make(chan struct{}),
	}
	go hints.run()
	return directoryObservation{registry: hints, events: files.Events, errors: files.Errors, hints: hints.events, hintErrors: hints.errors}, nil
}

func (d *directoryHints) Add(path string) error {
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
	fd, err := unix.Open(path, unix.O_EVTONLY|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
	if err != nil {
		return &os.PathError{Op: "watch", Path: path, Err: err}
	}
	event := unix.Kevent_t{Ident: uint64(fd), Filter: unix.EVFILT_VNODE, Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_ATTRIB | unix.NOTE_REVOKE}
	if _, err := unix.Kevent(d.queue, []unix.Kevent_t{event}, nil, nil); err != nil {
		return errors.Join(err, unix.Close(fd))
	}
	// Its old remove event may still be pending when the directory is rebound.
	if err := d.files.Remove(path); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		return errors.Join(err, unix.Close(fd))
	}
	if err := d.files.Add(path); err != nil {
		return errors.Join(err, unix.Close(fd))
	}
	d.paths[path] = fd
	return nil
}

func (d *directoryHints) Remove(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	fd, present := d.paths[path]
	if !present {
		return fsnotify.ErrNonExistentWatch
	}
	delete(d.paths, path)
	err := d.files.Remove(path)
	if errors.Is(err, fsnotify.ErrNonExistentWatch) {
		err = nil
	}
	return errors.Join(err, unix.Close(fd))
}

func (d *directoryHints) WatchList() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	paths := make([]string, 0, len(d.paths))
	for path := range d.paths {
		paths = append(paths, path)
	}
	return paths
}

func (d *directoryHints) run() {
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
			select {
			case d.errors <- err:
			case <-d.done:
			}
			return
		}
		for _, event := range events[:count] {
			select {
			case <-d.done:
				return
			default:
			}
			if event.Flags&unix.EV_ERROR != 0 && event.Data != 0 {
				select {
				case d.errors <- unix.Errno(event.Data):
				case <-d.done:
				}
				return
			}
			if event.Filter == unix.EVFILT_USER {
				continue
			}
			if event.Fflags&(unix.NOTE_DELETE|unix.NOTE_RENAME|unix.NOTE_REVOKE) != 0 {
				d.mu.Lock()
				var closeErr error
				for path, fd := range d.paths {
					if uint64(fd) == event.Ident {
						delete(d.paths, path)
						closeErr = unix.Close(fd)
						break
					}
				}
				d.mu.Unlock()
				if closeErr != nil {
					select {
					case d.errors <- closeErr:
					case <-d.done:
						return
					}
				}
			}
			select {
			case d.events <- fsnotify.Event{Op: fsnotify.Write}:
			case <-d.done:
				return
			}
		}
	}
}

func (d *directoryHints) Close() error {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		close(d.done)
		_, wakeErr := unix.Kevent(d.queue, []unix.Kevent_t{{Ident: shutdownHint, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}}, nil, nil)
		for errors.Is(wakeErr, unix.EINTR) {
			_, wakeErr = unix.Kevent(d.queue, []unix.Kevent_t{{Ident: shutdownHint, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER}}, nil, nil)
		}
		d.mu.Unlock()
		<-d.exited
		d.mu.Lock()
		defer d.mu.Unlock()
		var errs []error
		errs = append(errs, wakeErr)
		for path, fd := range d.paths {
			errs = append(errs, unix.Close(fd))
			delete(d.paths, path)
		}
		d.closeErr = errors.Join(append(errs, unix.Close(d.queue), d.files.Close())...)
	})
	return d.closeErr
}
