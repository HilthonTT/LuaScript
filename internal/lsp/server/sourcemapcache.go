package server

import (
	"slices"
	"sync"

	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// SourceMapCache is a cache of .templ file URIs to the source map.
type SourceMapCache struct {
	m              *sync.RWMutex
	uriToSourceMap map[string]*parser.SourceMap
}

func NewSourceMapCache() *SourceMapCache {
	return &SourceMapCache{
		m:              new(sync.RWMutex),
		uriToSourceMap: map[string]*parser.SourceMap{},
	}
}

func (fc *SourceMapCache) Set(uri string, m *parser.SourceMap) {
	fc.m.Lock()
	defer fc.m.Unlock()

	if m == nil {
		delete(fc.uriToSourceMap, uri)
		return
	}

	fc.uriToSourceMap[uri] = m
}

func (fc *SourceMapCache) Get(uri string) (*parser.SourceMap, bool) {
	fc.m.RLock()
	defer fc.m.RUnlock()

	m, ok := fc.uriToSourceMap[uri]
	return m, ok
}

func (fc *SourceMapCache) Delete(uri string) {
	fc.m.Lock()
	defer fc.m.Unlock()
	delete(fc.uriToSourceMap, uri)
}

func (fc *SourceMapCache) URIs() (uris []string) {
	fc.m.RLock()
	defer fc.m.RUnlock()
	uris = make([]string, len(fc.uriToSourceMap))
	var i int
	for k := range fc.uriToSourceMap {
		uris[i] = k
		i++
	}
	slices.Sort(uris)
	return uris
}
