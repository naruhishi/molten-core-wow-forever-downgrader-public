package main

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/selfupdate"
	"sync"
	"time"
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

type tabLifetime struct {
	mu         sync.Mutex
	timer      *time.Timer
	generation uint64
}

func (t *tabLifetime) resume() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.generation++
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}

func (t *tabLifetime) leave(stop func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.generation++
	generation := t.generation
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(3*time.Second, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.generation == generation {
			stop()
		}
	})
}
