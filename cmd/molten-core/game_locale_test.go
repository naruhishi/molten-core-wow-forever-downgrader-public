package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureGameLocalePreservesSettings(t *testing.T) {
	game := t.TempDir()
	if err := configureGameLocale(game, "enUS"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(game, "WTF", "Config.wtf")
	if err := os.WriteFile(path, []byte("SET gxWindow \"1\"\r\nSET textLocale \"enUS\"\r\nSET audioLocale \"enUS\"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := configureGameLocale(game, "esMX"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	want := "SET gxWindow \"1\"\r\nSET textLocale \"esMX\"\r\nSET audioLocale \"esMX\"\r\n"
	if err != nil || string(got) != want {
		t.Fatalf("Config.wtf = %q, want %q (%v)", got, want, err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.60.1.71037", "1.60.1.71037", 0},
		{"1.60.1.70959", "1.60.1.71037", -1},
		{"1.61.0.100", "1.60.9.99999", 1},
		{"1.9.0.1", "1.10.0.1", -1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
