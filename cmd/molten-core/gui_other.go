//go:build !windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/appwindow"
)

func launchGUI() bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	defer listener.Close()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	host := listener.Addr().String()
	origin := "http://" + host
	path := "/" + hex.EncodeToString(secret)
	job := &guiJob{snapshot: jobSnapshot{Status: "ready"}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	folderRequests := make(chan chan appwindow.FolderResult)
	var lifetime tabLifetime
	defer lifetime.resume()
	bridge := strings.ReplaceAll(unixBridge, "__RPC__", path+"/rpc")
	html := strings.Replace(string(page), "default-src 'none';", "default-src 'none'; connect-src 'self';", 1)
	html = strings.Replace(html, "<script>", "<script>"+bridge+"</script><script>", 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Frame-Options", "DENY")
			if r.Host != host {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodGet && r.URL.Path == path {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, html)
				return
			}
			if r.Method != http.MethodPost || r.URL.Path != path+"/rpc" || r.Header.Get("Origin") != origin {
				http.NotFound(w, r)
				return
			}
			var call struct{ Action, Mode, Version, Source, Destination string }
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&call) != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}
			var result any
			switch call.Action {
			case "start":
				if call.Mode != "validate" && call.Mode != "install" {
					http.Error(w, "Invalid mode", http.StatusBadRequest)
					return
				}
				if err := job.start(call.Mode, call.Version, call.Source, call.Destination); err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
			case "cancel":
				job.stop()
			case "attach":
				lifetime.resume()
			case "detach":
				lifetime.leave(stop)
			case "read":
				result = job.read()
			case "version":
				result = appVersion
			case "suggest":
				result = ""
				if home, err := os.UserHomeDir(); err == nil && versionPattern.MatchString(call.Version) {
					result = filepath.Join(home, "WoW-Forever-"+call.Version[strings.LastIndexByte(call.Version, '.')+1:])
				}
			case "folder":
				folder, err := pickDesktopFolder(r.Context(), ctx, folderRequests)
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				result = folder
			case "close":
				defer stop()
				var lifetime tabLifetime
				defer lifetime.resume()
			default:
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})}
	go server.Serve(listener)
	defer server.Close()
	url := origin + path
	if err := appwindow.Run(appwindow.Options{Title: "Molten Core - WoW Forever Downgrader", URL: url, Width: 940, Height: 810, Stop: ctx.Done(), FolderRequests: folderRequests}); err != nil {
		if runtime.GOOS == "linux" {
			fmt.Fprintln(os.Stderr, "Molten Core:", err)
			fmt.Fprintln(os.Stderr, "La ventana requiere GTK 3, WebKitGTK 4.1 o 4.0 y una sesión gráfica. / The window needs GTK 3, WebKitGTK 4.1 or 4.0, and a graphical session.")
			return true
		}
		open := "open"
		if err := exec.CommandContext(ctx, open, url).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Abra en el navegador / Open in browser:", url)
		}
		<-ctx.Done()
	}
	server.Close()
	job.stop()
	job.mu.Lock()
	done := job.done
	job.mu.Unlock()
	if done != nil {
		<-done
	}
	return true
}

func pickDesktopFolder(ctx, appContext context.Context, requests chan<- chan appwindow.FolderResult) (string, error) {
	if runtime.GOOS != "linux" {
		return unixPickFolder(ctx)
	}
	reply := make(chan appwindow.FolderResult, 1)
	select {
	case requests <- reply:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-appContext.Done():
		return "", appContext.Err()
	}
	select {
	case result := <-reply:
		return result.Path, result.Err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-appContext.Done():
		return "", appContext.Err()
	}
}

func unixPickFolder(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", "POSIX path of (choose folder)")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			details := strings.TrimSpace(string(exit.Stderr))
			if details == "" || strings.HasSuffix(details, "(-128)") {
				return "", nil
			}
			return "", errors.New(details)
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

const unixBridge = `
async function localRPC(Action, args={}){
 const response=await fetch('__RPC__',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({Action,...args})});
 if(!response.ok) throw Error(await response.text());
 return await response.json();
}
function startJob(Mode,Version,Source,Destination){return localRPC('start',{Mode,Version,Source,Destination});}
function cancelJob(){return localRPC('cancel');}
function readJob(){return localRPC('read');}
function getAppVersion(){return localRPC('version');}
function suggestDestination(Version){return localRPC('suggest',{Version});}
function pickFolder(){return localRPC('folder');}
function closeApp(){return localRPC('close');}
window.addEventListener('pageshow',()=>{localRPC('attach').catch(()=>{});});
window.addEventListener('pagehide',()=>{
 fetch('__RPC__',{method:'POST',keepalive:true,headers:{'Content-Type':'application/json'},body:'{"Action":"detach"}'}).catch(()=>{});
});
`
