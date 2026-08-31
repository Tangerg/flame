package runs

import "sync"

// sessionRunChanges wakes application continuations after a committed Run
// lifecycle change. It carries no Run state: callers must re-read the durable
// projection after every wake.
type sessionRunChanges struct {
	mu       sync.Mutex
	sessions map[string]*sessionRunObservation
}

type sessionRunObservation struct {
	changed   chan struct{}
	observers map[*sessionRunObserver]struct{}
}

// sessionRunObserver must have non-zero size: distinct pointers to zero-sized
// values are permitted to compare equal, but each observer is one exact map key.
type sessionRunObserver byte

func (s *sessionRunChanges) observe(sessionID string) (<-chan struct{}, func()) {
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = make(map[string]*sessionRunObservation)
	}
	observation := s.sessions[sessionID]
	if observation == nil {
		observation = &sessionRunObservation{
			changed:   make(chan struct{}),
			observers: make(map[*sessionRunObserver]struct{}),
		}
		s.sessions[sessionID] = observation
	}
	observer := new(sessionRunObserver)
	observation.observers[observer] = struct{}{}
	s.mu.Unlock()

	var once sync.Once
	return observation.changed, func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(observation.observers, observer)
			if len(observation.observers) == 0 && s.sessions[sessionID] == observation {
				delete(s.sessions, sessionID)
			}
		})
	}
}

func (s *sessionRunChanges) notify(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	observation := s.sessions[sessionID]
	if observation == nil {
		return
	}
	delete(s.sessions, sessionID)
	close(observation.changed)
}
