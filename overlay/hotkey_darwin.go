//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>

extern void nxHotkeyFired(void);

static EventHotKeyRef nxRef = NULL;
static EventHandlerRef nxHandler = NULL;

static OSStatus nxHandle(EventHandlerCallRef next, EventRef ev, void *ud) {
	nxHotkeyFired();
	return noErr;
}

static int nxRegister(unsigned int keycode, unsigned int mods) {
	if (nxHandler == NULL) {
		EventTypeSpec spec;
		spec.eventClass = kEventClassKeyboard;
		spec.eventKind = kEventHotKeyPressed;
		OSStatus st = InstallApplicationEventHandler(&nxHandle, 1, &spec, NULL, &nxHandler);
		if (st != noErr) return (int)st;
	}
	EventHotKeyID id;
	id.signature = 0x4e584154; // "NXAT"
	id.id = 1;
	return (int)RegisterEventHotKey(keycode, mods, id, GetApplicationEventTarget(), 0, &nxRef);
}

static void nxUnregister(void) {
	if (nxRef != NULL) {
		UnregisterEventHotKey(nxRef);
		nxRef = NULL;
	}
}
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/hotkey"
)

var hk struct {
	mu   sync.Mutex
	cb   func()
	last time.Time
}

// nxHotkeyFired é chamada pelo macOS (no laço principal do app) quando o atalho é pressionado.
//
//export nxHotkeyFired
func nxHotkeyFired() {
	hk.mu.Lock()
	cb := hk.cb
	debounce := time.Since(hk.last) < 400*time.Millisecond
	if !debounce {
		hk.last = time.Now()
	}
	hk.mu.Unlock()
	if cb != nil && !debounce {
		go cb()
	}
}

// setGlobalHotkey (re)registra o atalho global com o RegisterEventHotKey do macOS, que não pede
// permissão de acessibilidade. Win/Cmd vira a tecla Command e Alt vira Option. Spec vazio só remove o atual.
func setGlobalHotkey(spec string, onPress func()) error {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	C.nxUnregister()
	hk.cb = nil
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	combo, err := hotkey.ParseCombo(spec)
	if err != nil {
		return err
	}
	code, ok := combo.MacKeycode()
	if !ok {
		return fmt.Errorf("tecla desconhecida: %q", combo.Key)
	}
	var mods uint
	if combo.Super {
		mods |= 1 << 8 // cmdKey
	}
	if combo.Shift {
		mods |= 1 << 9 // shiftKey
	}
	if combo.Alt {
		mods |= 1 << 11 // optionKey
	}
	if combo.Ctrl {
		mods |= 1 << 12 // controlKey
	}
	if st := int(C.nxRegister(C.uint(code), C.uint(mods))); st != 0 {
		return fmt.Errorf("o macOS recusou o atalho %q (já está em uso?): código %d", spec, st)
	}
	hk.cb = onPress
	return nil
}
