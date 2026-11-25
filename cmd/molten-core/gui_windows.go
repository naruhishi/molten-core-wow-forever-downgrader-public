package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/appwindow"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/selfupdate"
	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
)

//go:embed installer.html
var page []byte

type jobSnapshot struct {
	Status          string
	Log             []string
	Release         string
	Received, Total int64
}

type guiJob struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	snapshot jobSnapshot
	client   *selfupdate.Client
	release  selfupdate.Release
	updated  bool
}

func (j *guiJob) start(mode, version, source, destination string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel != nil || j.updated {
		return fmt.Errorf("operation in progress or restart required")
	}
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version must contain four numeric components")
	}
	args := []string{"--validate", version}
	status := "validating"
	if mode == "install" {
		args = []string{"--target", version, source, destination}
		status = "installing"
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	j.cancel, j.done = cancel, done
	j.snapshot = jobSnapshot{Status: status}
	go func() {
		defer close(done)
		logf := func(format string, args ...any) {
			j.mu.Lock()
			defer j.mu.Unlock()
			if len(j.snapshot.Log) == 150 {
				j.snapshot.Log = j.snapshot.Log[1:]
			}
			j.snapshot.Log = append(j.snapshot.Log, fmt.Sprintf(format, args...))
		}
		err := run(ctx, nil, args, logf)
		result := "installed"
		if mode == "validate" {
			result = "validated"
		}
		if err != nil {
			result = "failed"
			if ctx.Err() != nil {
				result = "cancelled"
			} else {
				logf("Error: %v", err)
			}
		}
		cancel()
		j.mu.Lock()
		j.snapshot.Status, j.cancel = result, nil
		j.mu.Unlock()
	}()
	return nil
}

func (j *guiJob) update(download bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel != nil || j.updated {
		return errors.New("operation in progress or restart required")
	}
	if download && j.client == nil {
		return errors.New("check for updates first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	j.cancel, j.done = cancel, done
	j.snapshot = jobSnapshot{Status: "checkingUpdate"}
	if download {
		j.snapshot.Status = "downloadingUpdate"
	}
	go func() {
		defer close(done)
		defer cancel()
		var err error
		result := "upToDate"
		if download {
			err = applyRelease(ctx, j.client, j.release, func(got, total int64) {
				j.mu.Lock()
				defer j.mu.Unlock()
				j.snapshot.Received, j.snapshot.Total = got, total
			})
			result = "updateInstalled"
		} else {
			j.client, j.release, err = checkRelease(ctx)
			if err == nil && selfupdate.Newer(j.release.Tag, appVersion) {
				_, exe := j.release.Assets[updateEXE]
				_, sums := j.release.Assets[selfupdate.SumsName]
				if exe && sums {
					result = "updateAvailable"
				} else {
					result = "updateMissingAssets"
				}
			}
		}
		j.mu.Lock()
		defer j.mu.Unlock()
		if err != nil {
			result = "updateFailed"
			j.snapshot.Log = []string{err.Error()}
			if ctx.Err() != nil {
				result = "updateCancelled"
			}
		}
		if result == "updateAvailable" {
			j.snapshot.Release = j.release.Tag
		} else {
			j.client = nil
		}
		j.updated = result == "updateInstalled"
		j.snapshot.Status, j.cancel = result, nil
	}()
	return nil
}

func (j *guiJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel != nil {
		j.snapshot.Status = "cancelling"
		j.cancel()
	}
}

func (j *guiJob) read() jobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := j.snapshot
	result.Log = append([]string{}, result.Log...)
	return result
}

func launchGUI() bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const title = "Molten Core - WoW Forever Downgrader"
	if version, err := webviewloader.GetInstalledVersion(); version == "" || err != nil {
		appwindow.Alert(title, "Instala Microsoft Edge WebView2 Runtime para abrir la interfaz.\nInstall Microsoft Edge WebView2 Runtime to open the interface.")
		return true
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
	root, err := os.UserConfigDir()
	if err != nil {
		appwindow.Alert(title, err.Error())
		return true
	}
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:      filepath.Join(root, "MoltenCore", "WebView2"),
		AutoFocus:     true,
		WindowOptions: webview.WindowOptions{Title: title, Width: 940, Height: 810, Center: true, IconId: 1},
	})
	if w == nil {
		appwindow.Alert(title, "No se pudo abrir WebView2 / Could not open WebView2")
		return true
	}
	defer w.Destroy()
	j := &guiJob{snapshot: jobSnapshot{Status: "ready"}}
	w.Bind("startJob", j.start)
	w.Bind("checkUpdate", func() error { return j.update(false) })
	w.Bind("downloadUpdate", func() error { return j.update(true) })
	w.Bind("cancelJob", j.stop)
	w.Bind("readJob", j.read)
	w.Bind("getAppVersion", func() string { return appVersion })
	w.Bind("suggestDestination", func(version string) string {
		if !versionPattern.MatchString(version) {
			return ""
		}
		return filepath.Join(root, "WoW-Forever-"+version[strings.LastIndexByte(version, '.')+1:])
	})
	w.Bind("pickFolder", func() string { return pickFolder(uintptr(w.Window())) })
	w.SetSize(690, 580, webview.HintMin)
	w.Navigate("data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString(page))
	w.Run()
	j.stop()
	j.mu.Lock()
	done := j.done
	j.mu.Unlock()
	if done != nil {
		<-done
	}
	return true
}

type browseInfo struct {
	owner, root     uintptr
	display, title  *uint16
	flags           uint32
	callback, param uintptr
	image           int32
}

func pickFolder(owner uintptr) string {
	shell := syscall.NewLazyDLL("shell32.dll")
	ole := syscall.NewLazyDLL("ole32.dll")
	var display, path [260]uint16
	title, _ := syscall.UTF16PtrFromString("Selecciona una carpeta / Select a folder")
	info := browseInfo{owner: owner, display: &display[0], title: title, flags: 0x41}
	item, _, _ := shell.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if item == 0 {
		return ""
	}
	defer ole.NewProc("CoTaskMemFree").Call(item)
	ok, _, _ := shell.NewProc("SHGetPathFromIDListW").Call(item, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(path[:])
}
