package util

import "sync"

type ProtectedString struct {
	sync.RWMutex
	value string
}

func NewProtectedString() *ProtectedString {
	return &ProtectedString{}
}
func (s *ProtectedString) Get() string {
	s.RLock()
	defer s.RUnlock()
	return s.value
}

func (s *ProtectedString) Set(value string) {
	s.Lock()
	defer s.Unlock()
	s.value = value
}
