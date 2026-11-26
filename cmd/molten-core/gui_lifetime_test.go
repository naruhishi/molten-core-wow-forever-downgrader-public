package main

import (
	"testing"
	"time"
)

func TestTabCloseAndReload(t *testing.T) {
	var tab tabLifetime
	defer tab.resume()
	stopped := make(chan struct{}, 2)
	stop := func() { stopped <- struct{}{} }
	tab.leave(stop)
	tab.resume()
	select {
	case <-stopped:
		t.Fatal("reload closed the program")
	case <-time.After(3100 * time.Millisecond):
	}
	tab.leave(stop)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("closing the tab did not stop the program")
	}
}
