package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

type genericAlias struct {
	params []string
	target ast.TypeNode
}

type env struct {
	frames []frame

	aliases map[string]*Type

	generics map[string]*genericAlias
}

type frame struct {
	bindings map[string]*Type

	refined map[string]bool

	declared map[string]*Type
}

func newEnv() *env {
	return &env{
		frames:   []frame{{bindings: map[string]*Type{}}},
		aliases:  map[string]*Type{},
		generics: map[string]*genericAlias{},
	}
}

func (e *env) push() {
	e.frames = append(e.frames, frame{bindings: map[string]*Type{}})
}

func (e *env) pop() {
	if len(e.frames) == 0 {
		return
	}
	e.frames = e.frames[:len(e.frames)-1]
}

func (e *env) define(name string, t *Type) {
	if len(e.frames) == 0 {
		return
	}
	f := &e.frames[len(e.frames)-1]
	f.bindings[name] = t
	delete(f.refined, name)
	delete(f.declared, name)
}

func (e *env) defineRefined(name string, t *Type) {
	if len(e.frames) == 0 {
		return
	}
	f := &e.frames[len(e.frames)-1]
	if prev, exists := f.bindings[name]; exists && !f.refined[name] {
		if f.declared == nil {
			f.declared = map[string]*Type{}
		}
		f.declared[name] = prev
	}
	f.bindings[name] = t
	if f.refined == nil {
		f.refined = map[string]bool{}
	}
	f.refined[name] = true
}

func (e *env) lookupDeclared(name string) (*Type, bool) {
	for i := len(e.frames) - 1; i >= 0; i-- {
		f := e.frames[i]
		if t, ok := f.bindings[name]; ok {
			if f.refined[name] {
				if d, ok := f.declared[name]; ok {
					return d, true
				}
				continue
			}
			return t, true
		}
	}
	return nil, false
}

func (e *env) widenRefined(name string, t *Type) {
	for i := len(e.frames) - 1; i >= 0; i-- {
		f := &e.frames[i]
		if _, ok := f.bindings[name]; !ok {
			continue
		}
		if !f.refined[name] {
			return
		}
		f.bindings[name] = NewUnion(f.bindings[name], t)
		if _, hasDecl := f.declared[name]; hasDecl {
			return
		}
	}
}

func (e *env) visiblyRefinedNames() []string {
	var out []string
	seen := map[string]bool{}
	for i := len(e.frames) - 1; i >= 0; i-- {
		f := e.frames[i]
		for name := range f.bindings {
			if seen[name] {
				continue
			}
			seen[name] = true
			if f.refined[name] {
				out = append(out, name)
			}
		}
	}
	return out
}

func (e *env) visiblyRefined(name string) bool {
	for i := len(e.frames) - 1; i >= 0; i-- {
		f := e.frames[i]
		if _, ok := f.bindings[name]; ok {
			return f.refined[name]
		}
	}
	return false
}

func (e *env) dropRefinedInTop() {
	if len(e.frames) == 0 {
		return
	}
	f := &e.frames[len(e.frames)-1]
	for name := range f.refined {
		if d, ok := f.declared[name]; ok {
			f.bindings[name] = d
			delete(f.declared, name)
		} else {
			delete(f.bindings, name)
		}
		delete(f.refined, name)
	}
}

func (e *env) lookup(name string) (*Type, bool) {
	for i := len(e.frames) - 1; i >= 0; i-- {
		if t, ok := e.frames[i].bindings[name]; ok {
			return t, true
		}
	}
	return nil, false
}

func (e *env) shadowsGlobal(name string) bool {
	for i := len(e.frames) - 1; i >= 1; i-- {
		if _, ok := e.frames[i].bindings[name]; ok {
			return true
		}
	}
	return false
}

func (e *env) alias(name string) *Type {
	if t, ok := e.aliases[name]; ok {
		return t
	}
	return neverT
}
