//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/hotkey"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var hk struct {
	mu   sync.Mutex
	stop func()
}

// setGlobalHotkey (re)registra o atalho global no X11 (XGrabKey na janela raiz); spec vazio só remove o atual.
// No Wayland o sistema não deixa aplicativos capturarem teclas globais, então o atalho não é aceito ali.
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
	combo, err := hotkey.ParseCombo(spec)
	if err != nil {
		return err
	}
	keysym, ok := combo.X11Keysym()
	if !ok {
		return fmt.Errorf("tecla desconhecida: %q", combo.Key)
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return errors.New("no Wayland o sistema não permite atalhos globais; crie um atalho nas configurações do sistema que execute o comando nxaterm (ele traz a janela para a frente)")
	}
	if os.Getenv("DISPLAY") == "" {
		return errors.New("sem servidor gráfico X11 (DISPLAY vazio)")
	}

	x, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao servidor X11: %w", err)
	}
	setup := xproto.Setup(x)
	root := setup.DefaultScreen(x).Root
	count := byte(setup.MaxKeycode - setup.MinKeycode + 1)
	km, err := xproto.GetKeyboardMapping(x, setup.MinKeycode, count).Reply()
	if err != nil {
		x.Close()
		return fmt.Errorf("não foi possível ler o teclado: %w", err)
	}
	per := int(km.KeysymsPerKeycode)
	var code xproto.Keycode
	found := false
	for i, ks := range km.Keysyms {
		if per > 0 && i%per < 2 && uint32(ks) == keysym {
			code = setup.MinKeycode + xproto.Keycode(i/per)
			found = true
			break
		}
	}
	if !found {
		x.Close()
		return fmt.Errorf("a tecla %q não existe no teclado atual", combo.Key)
	}

	var mods uint16
	if combo.Ctrl {
		mods |= xproto.ModMaskControl
	}
	if combo.Alt {
		mods |= xproto.ModMask1
	}
	if combo.Shift {
		mods |= xproto.ModMaskShift
	}
	if combo.Super {
		mods |= xproto.ModMask4
	}
	// CapsLock e NumLock entram no estado do modificador: registra todas as combinações
	for _, extra := range []uint16{0, xproto.ModMaskLock, xproto.ModMask2, xproto.ModMaskLock | xproto.ModMask2} {
		if err := xproto.GrabKeyChecked(x, false, root, mods|extra, code, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			x.Close()
			return fmt.Errorf("o atalho %q já está em uso por outro programa (%v)", spec, err)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var last time.Time
		for {
			ev, xerr := x.WaitForEvent()
			if ev == nil && xerr == nil {
				return // conexão fechada
			}
			if ev == nil {
				continue // erro de uma requisição: segue esperando
			}
			if kp, ok := ev.(xproto.KeyPressEvent); ok && kp.Detail == code {
				// segurar a tecla repete o evento: só conta uma vez
				if time.Since(last) > 400*time.Millisecond {
					last = time.Now()
					go onPress()
				}
			}
		}
	}()
	hk.stop = func() {
		x.Close() // solta as teclas e faz WaitForEvent devolver nil
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}
