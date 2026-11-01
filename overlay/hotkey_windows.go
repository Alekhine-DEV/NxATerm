//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"nxaterm/internal/hotkey"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPeekMessage        = user32.NewProc("PeekMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012
	wmUser   = 0x0400
)

type hkMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

var hk struct {
	mu   sync.Mutex
	stop func()
}

// setGlobalHotkey (re)registra o atalho global; spec vazio apenas remove o atual.
// O Windows entrega WM_HOTKEY à thread que registrou, por isso há uma thread dedicada com laço de mensagens.
func setGlobalHotkey(spec string, onPress func()) error {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	if hk.stop != nil {
		hk.stop()
		hk.stop = nil
	}
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	mods, vk, err := hotkey.Parse(spec)
	if err != nil {
		return err
	}
	type started struct {
		tid uint32
		err error
	}
	ready := make(chan started, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid, _, _ := procGetCurrentThreadID.Call()
		var m hkMsg
		// cria a fila de mensagens da thread antes de qualquer PostThreadMessage
		procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, wmUser, wmUser, 0)
		r, _, e := procRegisterHotKey.Call(0, 1, uintptr(mods), uintptr(vk))
		if r == 0 {
			ready <- started{err: fmt.Errorf("o Windows recusou o atalho %q (já está em uso?): %v", spec, e)}
			return
		}
		ready <- started{tid: uint32(tid)}
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			if m.message == wmHotkey {
				go onPress()
			}
		}
		procUnregisterHotKey.Call(0, 1)
	}()
	st := <-ready
	if st.err != nil {
		<-done
		return st.err
	}
	hk.stop = func() {
		procPostThreadMessage.Call(uintptr(st.tid), wmQuit, 0, 0)
		<-done
	}
	return nil
}
