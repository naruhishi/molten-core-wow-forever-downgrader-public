//go:build linux && cgo

package appwindow

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdatomic.h>

typedef int (*Init)(int *, char ***);
typedef void *(*NewWindow)(int);
typedef void *(*NewChooser)(const char *, void *, int, const char *, ...);
typedef int (*RunDialog)(void *);
typedef char *(*GetFilename)(void *);
typedef void *(*RefSink)(void *);
typedef void (*Free)(void *);

extern uintptr_t nextFolderRequest(uintptr_t queue);
extern void finishFolderRequest(uintptr_t request, char *path, int failed);

typedef struct {
    void *gtk;
    void *window;
    uintptr_t queue;
} FolderPicker;
typedef void *(*NewWidget)(void);
typedef void (*SetText)(void *, const char *);
typedef void (*SetSize)(void *, int, int);
typedef void (*Add)(void *, void *);
typedef void (*Show)(void *);
typedef void (*Loop)(void);
typedef int (*Source)(void *);
typedef unsigned (*TimeoutAdd)(unsigned, Source, void *);
typedef int (*SourceRemove)(unsigned);
typedef unsigned long (*Connect)(void *, const char *, void (*)(void), void *, void *, int);

static atomic_int stopped;
static Show destroy;
static Loop quit;
static void on_destroy(void *widget, void *data) { quit(); }
static int on_stop(void *window) {
    if (atomic_load(&stopped)) destroy(window);
    return 1;
}
static int on_folder(void *data) {
    FolderPicker *picker = data;
    uintptr_t request = nextFolderRequest(picker->queue);
    if (!request) return 1;

    void *gtk = picker->gtk;
    NewChooser create = (NewChooser)dlsym(gtk, "gtk_file_chooser_dialog_new");
    RunDialog run = (RunDialog)dlsym(gtk, "gtk_dialog_run");
    GetFilename filename = (GetFilename)dlsym(gtk, "gtk_file_chooser_get_filename");
    RefSink ref_sink = (RefSink)dlsym(gtk, "g_object_ref_sink");
    Show unref = (Show)dlsym(gtk, "g_object_unref");
    Free release = (Free)dlsym(gtk, "g_free");
    if (!create || !run || !filename || !ref_sink || !unref || !release) {
        finishFolderRequest(request, NULL, 1);
        return 1;
    }

    enum { SELECT_FOLDER = 2, ACCEPT = -3, CANCEL = -6 };
    void *dialog = create("Molten Core", picker->window, SELECT_FOLDER,
                          "gtk-cancel", CANCEL, "gtk-open", ACCEPT, NULL);
    if (!dialog) {
        finishFolderRequest(request, NULL, 1);
        return 1;
    }
    typedef void (*SetFlag)(void *, int);
    SetFlag destroy_with_parent = (SetFlag)dlsym(gtk, "gtk_window_set_destroy_with_parent");
    if (destroy_with_parent) destroy_with_parent(dialog, 1);
    // The main window can close while gtk_dialog_run is in its nested loop.
    ref_sink(dialog);
    int response = run(dialog);
    char *path = response == ACCEPT ? filename(dialog) : NULL;
    destroy(dialog);
    unref(dialog);
    finishFolderRequest(request, path, response == ACCEPT && !path);
    release(path);
    return 1;
}

static void reset_stop(void) { atomic_store(&stopped, 0); }
static void request_stop(void) { atomic_store(&stopped, 1); }

static void *load_webkit(void) {
    void *handle = dlopen("libwebkit2gtk-4.1.so.0", RTLD_NOW | RTLD_LOCAL);
    if (!handle) handle = dlopen("libwebkit2gtk-4.0.so.37", RTLD_NOW | RTLD_LOCAL);
    return handle;
}

static int show_window(const char *title, const char *url, int width, int height, uintptr_t queue) {
    void *gtk = dlopen("libgtk-3.so.0", RTLD_NOW | RTLD_GLOBAL);
    if (!gtk) return 1;
    void *web = load_webkit();
    if (!web) return 2;
    Init init = (Init)dlsym(gtk, "gtk_init_check");
    NewWindow window_new = (NewWindow)dlsym(gtk, "gtk_window_new");
    SetText set_title = (SetText)dlsym(gtk, "gtk_window_set_title");
    SetSize set_size = (SetSize)dlsym(gtk, "gtk_window_set_default_size");
    Add add = (Add)dlsym(gtk, "gtk_container_add");
    Show show = (Show)dlsym(gtk, "gtk_widget_show_all");
    destroy = (Show)dlsym(gtk, "gtk_widget_destroy");
    Loop loop = (Loop)dlsym(gtk, "gtk_main");
    Connect connect = (Connect)dlsym(gtk, "g_signal_connect_data");
    NewWidget view_new = (NewWidget)dlsym(web, "webkit_web_view_new");
    SetText load_uri = (SetText)dlsym(web, "webkit_web_view_load_uri");
    TimeoutAdd schedule = (TimeoutAdd)dlsym(gtk, "g_timeout_add");
    SourceRemove remove_source = (SourceRemove)dlsym(gtk, "g_source_remove");
    quit = (Loop)dlsym(gtk, "gtk_main_quit");
    if (!init || !window_new || !set_title || !set_size || !add || !show || !destroy ||
        !loop || !connect || !view_new || !load_uri || !schedule || !remove_source || !quit) return 3;
    if (!init(NULL, NULL)) return 4;
    void *window = window_new(0);
    void *view = view_new();
    if (!window || !view) return 3;
    set_title(window, title);
    set_size(window, width, height);
    connect(window, "destroy", (void (*)(void))on_destroy, NULL, NULL, 0);
    add(window, view);
    load_uri(view, url);
    show(window);
    unsigned timer = schedule(100, on_stop, window);
    FolderPicker picker = {gtk, window, queue};
    unsigned folder_timer = queue ? schedule(100, on_folder, &picker) : 0;
    loop();
    if (folder_timer) remove_source(folder_timer);
    remove_source(timer);
    return 0;
}
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"
)

func run(o Options) error {
	title, url := C.CString(o.Title), C.CString(o.URL)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(url))

	// GTK owns the main thread; its timer observes the stop signal.
	C.reset_stop()
	if o.Stop != nil {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-o.Stop:
				C.request_stop()
			case <-done:
			}
		}()
	}
	var queue cgo.Handle
	if o.FolderRequests != nil {
		queue = cgo.NewHandle(o.FolderRequests)
		defer queue.Delete()
	}
	switch C.show_window(title, url, C.int(o.Width), C.int(o.Height), C.uintptr_t(queue)) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%w: GTK 3", ErrUnavailable)
	case 2:
		return fmt.Errorf("%w: WebKitGTK 4.1 or 4.0", ErrUnavailable)
	case 4:
		return fmt.Errorf("%w: no graphical desktop", ErrUnavailable)
	default:
		return fmt.Errorf("%w: GTK/WebKit initialization failed", ErrUnavailable)
	}
}

func alert(string, string) bool { return false }

//export nextFolderRequest
func nextFolderRequest(queue C.uintptr_t) C.uintptr_t {
	requests := cgo.Handle(queue).Value().(<-chan chan FolderResult)
	select {
	case reply := <-requests:
		return C.uintptr_t(cgo.NewHandle(reply))
	default:
		return 0
	}
}

//export finishFolderRequest
func finishFolderRequest(request C.uintptr_t, path *C.char, failed C.int) {
	handle := cgo.Handle(request)
	defer handle.Delete()
	result := FolderResult{Path: C.GoString(path)}
	if failed != 0 {
		result.Err = fmt.Errorf("%w: GTK folder picker", ErrUnavailable)
	}
	handle.Value().(chan FolderResult) <- result
}
