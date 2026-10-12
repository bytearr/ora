//go:build linux && cgo && !nogui

package gui

// #cgo pkg-config: gtk4 x11
// #cgo CFLAGS: -Wno-deprecated-declarations
// #include <gtk/gtk.h>
// #include <gdk/x11/gdkx.h>
//
// static int ora_x11_workarea(uintptr_t m, int *x, int *y, int *w, int *h) {
// 	GdkMonitor *mon = (GdkMonitor *)m;
// 	if (!GDK_IS_X11_MONITOR(mon)) return 0;
// 	GdkRectangle r;
// 	gdk_x11_monitor_get_workarea(mon, &r);
// 	*x = r.x; *y = r.y; *w = r.width; *h = r.height;
// 	return 1;
// }
//
// static int ora_x11_move(uintptr_t s, int x, int y) {
// 	GdkSurface *surf = (GdkSurface *)s;
// 	if (!GDK_IS_X11_SURFACE(surf)) return 0;
// 	int scale = gdk_surface_get_scale_factor(surf);
// 	Display *d = gdk_x11_display_get_xdisplay(gdk_surface_get_display(surf));
// 	XMoveWindow(d, gdk_x11_surface_get_xid(surf), x * scale, y * scale);
// 	XFlush(d);
// 	return 1;
// }
import "C"

import (
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
)

// x11Workarea is the work area of mon (without panels) in logical pixels;
// false when mon is not an X11 monitor.
func x11Workarea(mon *gdk.Monitor) (x, y, w, h int, ok bool) {
	var cx, cy, cw, ch C.int
	if C.ora_x11_workarea(C.uintptr_t(coreglib.InternObject(mon).Native()), &cx, &cy, &cw, &ch) == 0 {
		return 0, 0, 0, 0, false
	}
	return int(cx), int(cy), int(cw), int(ch), true
}

// x11Move puts the top-left corner of an X11 surface at x, y in logical
// pixels; false when s is not an X11 surface (Wayland cannot be moved).
func x11Move(s gdk.Surfacer, x, y int) bool {
	return C.ora_x11_move(C.uintptr_t(coreglib.InternObject(s).Native()), C.int(x), C.int(y)) != 0
}
