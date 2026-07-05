package server

import "sync"

// document is the server's in-memory view of one open source file. The full
// text is kept because the sync mode negotiated in Initialize is
// TextDocumentSyncKindFull — every change ships the complete document, so
// there is no incremental-patch bookkeeping to do.
type document struct {
	uri     string
	text    string
	version int32
}

// documentStore is a concurrency-safe map of URI -> open document. The LSP
// dispatch loop is single-goroutine per connection today, but diagnostics may
// be published from helper goroutines, so guard access with a mutex.
type documentStore struct {
	mu   sync.RWMutex
	docs map[string]*document
}

func newDocumentStore() *documentStore {
	return &documentStore{docs: make(map[string]*document)}
}

// open records a freshly opened document, replacing any prior entry.
func (s *documentStore) open(uri, text string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = &document{uri: uri, text: text, version: version}
}

// update overwrites the full text of an open document. Unknown URIs are
// inserted so a stray didChange before didOpen doesn't lose content.
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

// close forgets a document.
func (s *documentStore) close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

// get returns the current text of a document and whether it is open.
func (s *documentStore) get(uri string) (text string, version int32, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[uri]
	if !ok {
		return "", 0, false
	}
	return d.text, d.version, true
}
