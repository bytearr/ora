//go:build linux

package gui

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"ora/core/config"
)

const (
	portalName = "org.freedesktop.portal.Desktop"
	portalPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	shortcutIf = "org.freedesktop.portal.GlobalShortcuts"
	requestIf  = "org.freedesktop.portal.Request"
	toggleID   = "toggle"
)

// errNoPortal: the desktop offers no GlobalShortcuts portal.
var errNoPortal = errors.New("the desktop has no global shortcut portal")

// appPath is the D-Bus object path GApplication uses for appID.
func appPath() dbus.ObjectPath {
	return dbus.ObjectPath("/" + strings.ReplaceAll(appID, ".", "/"))
}

// activationData passes the launcher's activation token on, so the
// compositor lets the running window take the focus.
func activationData() map[string]dbus.Variant {
	d := map[string]dbus.Variant{}
	if t := os.Getenv("XDG_ACTIVATION_TOKEN"); t != "" {
		d["activation-token"] = dbus.MakeVariant(t)
	}
	if t := os.Getenv("DESKTOP_STARTUP_ID"); t != "" {
		d["desktop-startup-id"] = dbus.MakeVariant(t)
	}
	return d
}

// activateRunning hands over to a running window: it asks it to show or
// hide itself (toggle), or does nothing when hidden is set. running is
// false when there is no window or no session bus.
func activateRunning(hidden bool) (running bool, err error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, nil
	}
	var owned bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, appID).Store(&owned); err != nil || !owned {
		return false, nil
	}
	if hidden {
		return true, nil
	}
	call := conn.Object(appID, appPath()).Call("org.freedesktop.Application.Activate", 0, activationData())
	if call.Err != nil {
		return true, fmt.Errorf("activate the running window: %w", call.Err)
	}
	return true, nil
}

// shortcutTrigger is settings.hotkey in the XDG shortcuts notation the
// portal takes as preferred trigger, e.g. "CTRL+ALT+space". A two-key
// combo such as "LWin & LAlt" has no such form: "" lets the desktop ask.
func shortcutTrigger(h config.Hotkey) string {
	if h.IsCombo() {
		return ""
	}
	key := keysymName(h.Trigger)
	if key == "" {
		return ""
	}
	var parts []string
	for _, m := range []struct {
		side config.Side
		name string
	}{{h.Ctrl, "CTRL"}, {h.Alt, "ALT"}, {h.Shift, "SHIFT"}, {h.Win, "LOGO"}} {
		if m.side != config.SideNone {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, key), "+")
}

var keysyms = map[uint32]string{
	0x20: "space", 0x09: "Tab", 0x0D: "Return", 0x1B: "Escape", 0x08: "BackSpace",
	0x2E: "Delete", 0x2D: "Insert", 0x24: "Home", 0x23: "End", 0x21: "Page_Up", 0x22: "Page_Down",
	0x26: "Up", 0x28: "Down", 0x25: "Left", 0x27: "Right", 0x5D: "Menu", 0x14: "Caps_Lock",
	0x91: "Scroll_Lock", 0x90: "Num_Lock", 0x2C: "Print", 0x13: "Pause",
	0x6A: "KP_Multiply", 0x6B: "KP_Add", 0x6D: "KP_Subtract", 0x6E: "KP_Decimal", 0x6F: "KP_Divide",
}

// keysymName maps a Windows virtual key to its xkb keysym name.
func keysymName(vk uint32) string {
	switch {
	case vk >= 'A' && vk <= 'Z':
		return string(rune(vk - 'A' + 'a'))
	case vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= 0x60 && vk <= 0x69:
		return fmt.Sprintf("KP_%d", vk-0x60)
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", vk-0x70+1)
	}
	return keysyms[vk]
}

func token() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ora" + hex.EncodeToString(b)
}

// portalRequest calls a portal method that answers through a Request
// object and waits for its Response. opts gets the handle_token.
func portalRequest(conn *dbus.Conn, method string, timeout time.Duration, opts map[string]dbus.Variant, args ...any) (map[string]dbus.Variant, error) {
	tok := token()
	opts["handle_token"] = dbus.MakeVariant(tok)
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	path := dbus.ObjectPath(fmt.Sprintf("%s/request/%s/%s", portalPath, sender, tok))
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(requestIf), dbus.WithMatchMember("Response")}
	if err := conn.AddMatchSignal(match...); err != nil {
		return nil, err
	}
	defer conn.RemoveMatchSignal(match...)
	ch := make(chan *dbus.Signal, 4)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	if call := conn.Object(portalName, portalPath).Call(method, 0, append(args, opts)...); call.Err != nil {
		return nil, call.Err
	}
	deadline := time.After(timeout)
	for {
		select {
		case sig := <-ch:
			if sig.Path != path || sig.Name != requestIf+".Response" || len(sig.Body) < 2 {
				continue
			}
			code, _ := sig.Body[0].(uint32)
			res, _ := sig.Body[1].(map[string]dbus.Variant)
			switch code {
			case 0:
				return res, nil
			case 1:
				return nil, errors.New("cancelled")
			default:
				return nil, fmt.Errorf("%s failed", method)
			}
		case <-deadline:
			return nil, fmt.Errorf("%s: no answer within %s", method, timeout)
		}
	}
}

// bindShortcut asks the GlobalShortcuts portal for the toggle shortcut and
// calls activated (off the UI thread) with the activation token each time
// it fires. The desktop may show a dialog to confirm or pick the keys, so
// it waits long; it returns once the shortcut is bound.
func bindShortcut(trigger string, activated func(token string)) (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", err
	}
	portal := conn.Object(portalName, portalPath)
	if _, err := portal.GetProperty(shortcutIf + ".version"); err != nil {
		return "", errNoPortal
	}
	// Unsandboxed apps tell the portal who they are (xdg-desktop-portal 1.19+).
	portal.Call("org.freedesktop.host.portal.Registry.Register", 0, appID, map[string]dbus.Variant{})

	res, err := portalRequest(conn, shortcutIf+".CreateSession", 30*time.Second,
		map[string]dbus.Variant{"session_handle_token": dbus.MakeVariant(token())})
	if err != nil {
		return "", fmt.Errorf("global shortcut session: %w", err)
	}
	var session dbus.ObjectPath
	switch v := res["session_handle"].Value().(type) {
	case string:
		session = dbus.ObjectPath(v)
	case dbus.ObjectPath:
		session = v
	}
	if session == "" {
		return "", errors.New("global shortcut session: no handle")
	}

	match := []dbus.MatchOption{dbus.WithMatchInterface(shortcutIf), dbus.WithMatchMember("Activated")}
	if err := conn.AddMatchSignal(match...); err != nil {
		return "", err
	}
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	go func() {
		for sig := range ch {
			if sig.Name != shortcutIf+".Activated" || len(sig.Body) < 2 {
				continue
			}
			if s, _ := sig.Body[0].(dbus.ObjectPath); s != session {
				continue
			}
			if id, _ := sig.Body[1].(string); id != toggleID {
				continue
			}
			tok := ""
			if len(sig.Body) >= 4 {
				if o, ok := sig.Body[3].(map[string]dbus.Variant); ok {
					tok, _ = o["activation_token"].Value().(string)
				}
			}
			activated(tok)
		}
	}()

	props := map[string]dbus.Variant{"description": dbus.MakeVariant("Show or hide the ora window")}
	if trigger != "" {
		props["preferred_trigger"] = dbus.MakeVariant(trigger)
	}
	type shortcut struct {
		ID    string
		Props map[string]dbus.Variant
	}
	res, err = portalRequest(conn, shortcutIf+".BindShortcuts", 10*time.Minute, map[string]dbus.Variant{},
		session, []shortcut{{toggleID, props}}, "")
	if err != nil {
		return "", fmt.Errorf("bind the global shortcut: %w", err)
	}
	return boundTrigger(res), nil
}

// boundTrigger is the trigger the desktop reports for the toggle shortcut.
func boundTrigger(res map[string]dbus.Variant) string {
	list, ok := res["shortcuts"].Value().([][]any)
	if !ok {
		return ""
	}
	for _, s := range list {
		if len(s) < 2 || s[0] != toggleID {
			continue
		}
		if p, ok := s[1].(map[string]dbus.Variant); ok {
			d, _ := p["trigger_description"].Value().(string)
			return d
		}
	}
	return ""
}

// accentColor is the desktop accent from the Settings portal as #rrggbb,
// or "" if it has none.
func accentColor() string {
	conn, err := dbus.SessionBus()
	if err != nil {
		return ""
	}
	var v dbus.Variant
	err = conn.Object(portalName, portalPath).Call("org.freedesktop.portal.Settings.ReadOne", 0,
		"org.freedesktop.appearance", "accent-color").Store(&v)
	if err != nil {
		return ""
	}
	return rgbHex(v.Value())
}

// rgbHex turns the portal's (ddd), channels 0..1, into #rrggbb. Values out
// of range mean "no accent".
func rgbHex(v any) string {
	if inner, ok := v.(dbus.Variant); ok {
		v = inner.Value()
	}
	c, ok := v.([]any)
	if !ok || len(c) != 3 {
		return ""
	}
	var b [3]int
	for i, x := range c {
		f, ok := x.(float64)
		if !ok || f < 0 || f > 1 {
			return ""
		}
		b[i] = int(f*255 + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", b[0], b[1], b[2])
}
