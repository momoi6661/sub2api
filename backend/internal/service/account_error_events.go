package service

import "sync"

// AccountErrorEvents connects a successfully persisted account error to recovery.
// One shared instance serves both gateway and admin repositories.
type AccountErrorEvents struct {
	mu     sync.RWMutex
	notify func(int64)
}

func NewAccountErrorEvents() *AccountErrorEvents { return &AccountErrorEvents{} }

func (e *AccountErrorEvents) SetHandler(notify func(int64)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.notify = notify
}

func (e *AccountErrorEvents) Notify(id int64, message string) {
	if e == nil || !isOneClickAuthFailureMessage(message) {
		return
	}
	e.mu.RLock()
	notify := e.notify
	e.mu.RUnlock()
	if notify != nil {
		notify(id)
	}
}
