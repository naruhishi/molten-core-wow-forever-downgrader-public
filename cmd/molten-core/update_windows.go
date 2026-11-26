package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/selfupdate"
)

var updateChannel = "public"

const updateEXE = "Molten-Core-WoW-Forever-Downgrader.exe"

type releaseTransport struct{ token, path string }

func (t releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, errors.New("release download requires HTTPS")
	}
	req = req.Clone(req.Context())
	if req.URL.Host == "api.github.com" &&
		strings.HasPrefix(req.URL.Path, t.path) {
		if t.token != "" {
			req.Header.Set("Authorization", "Bearer "+t.token)
		}
		if strings.HasPrefix(req.URL.Path, t.path+"assets/") {
			req.Header.Set("Accept", "application/octet-stream")
		}
	} else {
		req.Header.Del("Authorization")
		if req.URL.Host != "release-assets.githubusercontent.com" && req.URL.Host != "objects.githubusercontent.com" {
			return nil, errors.New("unexpected release asset host")
		}
	}
	return http.DefaultTransport.RoundTrip(req)
}

func checkRelease(ctx context.Context) (*selfupdate.Client, selfupdate.Release, error) {
	var empty selfupdate.Release
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	repo := "naruhishi/molten-core-wow-forever-downgrader"
	var token string
	switch updateChannel {
	case "private":
		command := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com")
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		out, err := command.Output()
		token = strings.TrimSpace(string(out))
		if err != nil || token == "" {
			return nil, empty, errors.New("Instala GitHub CLI e inicia sesión con gh auth login / Install GitHub CLI and sign in with gh auth login.")
		}
	case "public":
		repo += "-public"
	default:
		return nil, empty, errors.New("unknown update channel")
	}
	path := "/repos/" + repo + "/releases/"
	client := &selfupdate.Client{API: "https://api.github.com" + path + "latest", UseAssetAPI: true,
		HTTP: &http.Client{Transport: releaseTransport{token: token, path: path}}, UserAgent: "Molten-Core/" + appVersion}
	rel, err := client.Latest(ctx)
	if err != nil {
		return nil, empty, err
	}
	return client, rel, nil
}

func applyRelease(ctx context.Context, client *selfupdate.Client, rel selfupdate.Release, progress func(int64, int64)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target, err := selfupdate.TargetFor(exe)
	if err != nil {
		return err
	}
	if err := target.Writable(); err != nil {
		return err
	}
	sums, err := client.Sums(ctx, rel)
	if err != nil {
		return err
	}
	if sums[updateEXE] == "" {
		return errors.New("SHA256SUMS.txt does not list the executable")
	}
	staged, err := client.Download(ctx, rel.Assets[updateEXE], sums[updateEXE], target.Dir(), progress)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	if err := ctx.Err(); err != nil {
		return err
	}
	return target.Replace(staged, func(program string) error {
		probeCtx, stop := context.WithTimeout(ctx, 20*time.Second)
		defer stop()
		cmd := exec.CommandContext(probeCtx, program, "--app-version")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(out)) != rel.Tag {
			return fmt.Errorf("release version mismatch: expected %s", rel.Tag)
		}
		return ctx.Err()
	})
}
