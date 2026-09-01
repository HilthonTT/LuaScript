package server

import "sync"

type document struct {
	uri     string
	text    string
	version int32
}

type documentStore struct {
	mu   sync.RWMutex
	docs map[string]*document
}

func newDocumentStore() *documentStore {
	return &documentStore{docs: make(map[string]*document)}
}

func (s *documentStore) open(uri, text string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = &document{uri: uri, text: text, version: version}
}

func (s *documentStore) update(uri, text string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[uri]
	if !ok {
		s.docs[uri] = &document{uri: uri, text: text, version: version}
		return
	}
	d.text = text
	d.version = version
}

func (s *documentStore) close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

func (s *documentStore) get(uri string) (text string, version int32, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[uri]
	if !ok {
		return "", 0, false
	}
	return d.text, d.version, true
}
