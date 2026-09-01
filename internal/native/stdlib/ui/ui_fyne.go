//go:build luascript_ui

package ui

import (
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/hilthontt/luascript/internal/vm"
)

var (
	appMu   sync.Mutex
	fyneApp fyne.App
)

func getApp() fyne.App {
	appMu.Lock()
	defer appMu.Unlock()
	if fyneApp == nil {
		fyneApp = fyneapp.New()
	}
	return fyneApp
}

var (
	regMu          sync.Mutex
	widgetRegistry = map[*vm.Table]fyne.CanvasObject{}
)

func registerWidget(t *vm.Table, obj fyne.CanvasObject) {
	regMu.Lock()
	widgetRegistry[t] = obj
	regMu.Unlock()
}

func lookupWidget(t *vm.Table) (fyne.CanvasObject, bool) {
	regMu.Lock()
	defer regMu.Unlock()
	obj, ok := widgetRegistry[t]
	return obj, ok
}

func funcArg(name string, n int, args []vm.Value) vm.Value {
	if n < 1 || n > len(args) {
		panic(vm.Errorf("bad argument #%d to '%s' (function expected)", n, name))
	}
	switch args[n-1].(type) {
	case *vm.Closure, *vm.GoFunc:
		return args[n-1]
	}
	panic(vm.Errorf("bad argument #%d to '%s' (function expected, got %s)", n, name, vm.TypeName(args[n-1])))
}

func withMethods(methods *vm.Table) *vm.Table {
	t := vm.NewTable(0, 0)
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	t.SetMetatable(mt)
	return t
}

func uiLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("VERSION", "0.1.0")

	methods := vm.NewTable(0, 8)

	methods.Set("new_window", &vm.GoFunc{Name: "ui:new_window", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		title := vm.OptString("ui.new_window", 1, args, "luascript")
		w := vm.OptInt("ui.new_window", 2, args, 480)
		h := vm.OptInt("ui.new_window", 3, args, 320)
		win := getApp().NewWindow(title)
		win.Resize(fyne.NewSize(float32(w), float32(h)))
		return []vm.Value{wrapWindow(win)}
	}})

	methods.Set("new_label", &vm.GoFunc{Name: "ui:new_label", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		text := vm.OptString("ui.new_label", 1, args, "")
		return []vm.Value{wrapLabel(widget.NewLabel(text))}
	}})

	methods.Set("new_button", &vm.GoFunc{Name: "ui:new_button", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		text := vm.OptString("ui.new_button", 1, args, "")
		btn := widget.NewButton(text, nil)
		return []vm.Value{wrapButton(btn)}
	}})

	methods.Set("new_entry", &vm.GoFunc{Name: "ui:new_entry", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		ph := vm.OptString("ui.new_entry", 1, args, "")
		e := widget.NewEntry()
		if ph != "" {
			e.SetPlaceHolder(ph)
		}
		return []vm.Value{wrapEntry(e)}
	}})

	methods.Set("new_vbox", &vm.GoFunc{Name: "ui:new_vbox", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		objs := collectChildren("ui.new_vbox", args)
		return []vm.Value{wrapContainer(container.NewVBox(objs...))}
	}})

	methods.Set("new_hbox", &vm.GoFunc{Name: "ui:new_hbox", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		objs := collectChildren("ui.new_hbox", args)
		return []vm.Value{wrapContainer(container.NewHBox(objs...))}
	}})

	methods.Set("run", &vm.GoFunc{Name: "ui:run", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		getApp().Run()
		return nil
	}})

	methods.Set("quit", &vm.GoFunc{Name: "ui:quit", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		appMu.Lock()
		a := fyneApp
		appMu.Unlock()
		if a != nil {
			a.Quit()
		}
		return nil
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	mod.SetMetatable(mt)
	return []vm.Value{mod}
}

func collectChildren(site string, args []vm.Value) []fyne.CanvasObject {
	t := vm.TableArg(site, 1, args)
	n := t.Len()
	out := make([]fyne.CanvasObject, 0, n)
	for i := int64(1); i <= n; i++ {
		v := t.Get(i)
		child, ok := v.(*vm.Table)
		if !ok {
			panic(vm.Errorf("bad child #%d to '%s' (widget expected, got %s)", i, site, vm.TypeName(v)))
		}
		obj, ok := lookupWidget(child)
		if !ok {
			panic(vm.Errorf("bad child #%d to '%s' (not a registered widget)", i, site))
		}
		out = append(out, obj)
	}
	return out
}

func wrapWindow(w fyne.Window) *vm.Table {
	methods := vm.NewTable(0, 8)

	methods.Set("set_content", &vm.GoFunc{Name: "window:set_content", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("window:set_content", 1, args)
		if len(args) < 2 {
			panic(vm.Errorf("bad argument #2 to 'window:set_content' (widget expected)"))
		}
		child, ok := args[1].(*vm.Table)
		if !ok {
			panic(vm.Errorf("bad argument #2 to 'window:set_content' (widget expected, got %s)", vm.TypeName(args[1])))
		}
		obj, ok := lookupWidget(child)
		if !ok {
			panic(vm.Errorf("bad argument #2 to 'window:set_content' (not a registered widget)"))
		}
		w.SetContent(obj)
		return nil
	}})

	methods.Set("show", &vm.GoFunc{Name: "window:show", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		w.Show()
		return nil
	}})
	methods.Set("hide", &vm.GoFunc{Name: "window:hide", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		w.Hide()
		return nil
	}})
	methods.Set("close", &vm.GoFunc{Name: "window:close", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		w.Close()
		return nil
	}})
	methods.Set("set_title", &vm.GoFunc{Name: "window:set_title", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("window:set_title", 1, args)
		title := vm.StringArg("window:set_title", 2, args)
		w.SetTitle(title)
		return nil
	}})
	methods.Set("resize", &vm.GoFunc{Name: "window:resize", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("window:resize", 1, args)
		wpx := vm.IntArg("window:resize", 2, args)
		hpx := vm.IntArg("window:resize", 3, args)
		w.Resize(fyne.NewSize(float32(wpx), float32(hpx)))
		return nil
	}})
	methods.Set("show_and_run", &vm.GoFunc{Name: "window:show_and_run", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w.ShowAndRun()
		return nil
	}})

	t := withMethods(methods)
	return t
}

func wrapLabel(l *widget.Label) *vm.Table {
	methods := vm.NewTable(0, 2)
	methods.Set("set_text", &vm.GoFunc{Name: "label:set_text", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("label:set_text", 1, args)
		s := vm.StringArg("label:set_text", 2, args)
		l.SetText(s)
		return nil
	}})
	methods.Set("get_text", &vm.GoFunc{Name: "label:get_text", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{l.Text}
	}})
	t := withMethods(methods)
	registerWidget(t, l)
	return t
}

func wrapButton(b *widget.Button) *vm.Table {
	methods := vm.NewTable(0, 4)
	methods.Set("set_text", &vm.GoFunc{Name: "button:set_text", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("button:set_text", 1, args)
		s := vm.StringArg("button:set_text", 2, args)
		b.SetText(s)
		return nil
	}})
	methods.Set("get_text", &vm.GoFunc{Name: "button:get_text", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{b.Text}
	}})
	methods.Set("on_click", &vm.GoFunc{Name: "button:on_click", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("button:on_click", 1, args)
		cb := funcArg("button:on_click", 2, args)
		b.OnTapped = func() {
			v.CallValue(cb, nil, 0)
		}
		return nil
	}})
	methods.Set("disable", &vm.GoFunc{Name: "button:disable", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		b.Disable()
		return nil
	}})
	t := withMethods(methods)
	registerWidget(t, b)
	return t
}

func wrapEntry(e *widget.Entry) *vm.Table {
	methods := vm.NewTable(0, 4)
	methods.Set("set_text", &vm.GoFunc{Name: "entry:set_text", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("entry:set_text", 1, args)
		s := vm.StringArg("entry:set_text", 2, args)
		e.SetText(s)
		return nil
	}})
	methods.Set("get_text", &vm.GoFunc{Name: "entry:get_text", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{e.Text}
	}})
	methods.Set("on_changed", &vm.GoFunc{Name: "entry:on_changed", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("entry:on_changed", 1, args)
		cb := funcArg("entry:on_changed", 2, args)
		e.OnChanged = func(s string) {
			v.CallValue(cb, []vm.Value{s}, 0)
		}
		return nil
	}})
	methods.Set("on_submitted", &vm.GoFunc{Name: "entry:on_submitted", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("entry:on_submitted", 1, args)
		cb := funcArg("entry:on_submitted", 2, args)
		e.OnSubmitted = func(s string) {
			v.CallValue(cb, []vm.Value{s}, 0)
		}
		return nil
	}})
	t := withMethods(methods)
	registerWidget(t, e)
	return t
}

func wrapContainer(c *fyne.Container) *vm.Table {
	methods := vm.NewTable(0, 2)
	methods.Set("add", &vm.GoFunc{Name: "container:add", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("container:add", 1, args)
		if len(args) < 2 {
			panic(vm.Errorf("bad argument #2 to 'container:add' (widget expected)"))
		}
		child, ok := args[1].(*vm.Table)
		if !ok {
			panic(vm.Errorf("bad argument #2 to 'container:add' (widget expected, got %s)", vm.TypeName(args[1])))
		}
		obj, ok := lookupWidget(child)
		if !ok {
			panic(vm.Errorf("bad argument #2 to 'container:add' (not a registered widget)"))
		}
		c.Add(obj)
		c.Refresh()
		return nil
	}})
	methods.Set("remove_all", &vm.GoFunc{Name: "container:remove_all", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		c.RemoveAll()
		c.Refresh()
		return nil
	}})
	t := withMethods(methods)
	registerWidget(t, c)
	return t
}
