//go:build linux && cgo && !nogui

package gui

// #cgo pkg-config: gtk4 gtk4-layer-shell-0
// #include <gtk/gtk.h>
// #include <gtk4-layer-shell.h>
//
// static gboolean ora_layer_supported(void) { return gtk_layer_is_supported(); }
//
// static void ora_layer_init(uintptr_t w) {
// 	GtkWindow *win = (GtkWindow *)w;
// 	gtk_layer_init_for_window(win);
// 	gtk_layer_set_namespace(win, "ora");
// 	gtk_layer_set_layer(win, GTK_LAYER_SHELL_LAYER_OVERLAY);
// 	gtk_layer_set_exclusive_zone(win, -1);
// 	gtk_layer_set_keyboard_mode(win, GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
// 	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_TOP, TRUE);
// 	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_BOTTOM, TRUE);
// 	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_LEFT, TRUE);
// 	gtk_layer_set_anchor(win, GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
// }
//
// static void ora_layer_monitor(uintptr_t w, uintptr_t m) {
// 	gtk_layer_set_monitor((GtkWindow *)w, (GdkMonitor *)m);
// }
import "C"

import (
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// layerSupported says whether the compositor speaks wlr-layer-shell
// (sway, Hyprland, KDE, niri, COSMIC, ...; not GNOME, not X11).
func layerSupported() bool { return C.ora_layer_supported() != 0 }

// layerInit makes win a transparent overlay over a whole monitor that
// takes the keyboard while it is mapped. The card in it is placed like
// the Windows window; a click beside the card hides it. Call before the
// window is realized.
func layerInit(win *gtk.Window) {
	C.ora_layer_init(C.uintptr_t(coreglib.InternObject(win).Native()))
}

// layerMonitor puts the overlay on mon; nil lets the compositor pick the
// output that has the focus.
func layerMonitor(win *gtk.Window, mon *gdk.Monitor) {
	var m uintptr
	if mon != nil {
		m = coreglib.InternObject(mon).Native()
	}
	C.ora_layer_monitor(C.uintptr_t(coreglib.InternObject(win).Native()), C.uintptr_t(m))
}
