package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/casc"
)

func configureGameLocale(gameDir, locale string) error {
	dir := filepath.Join(gameDir, "WTF")
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a regular directory", dir)
	}
	path := filepath.Join(dir, "Config.wtf")
	var data []byte
	mode := os.FileMode(0o600)
	info, err = os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		mode = info.Mode().Perm()
		data, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	case !os.IsNotExist(err):
		return err
	}
	newline := "\n"
	if strings.Contains(string(data), "\r\n") {
		newline = "\r\n"
	}
	text := strings.TrimSuffix(string(data), newline)
	var lines []string
	if text != "" {
		for _, line := range strings.Split(text, newline) {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(fields[0], "SET") &&
				(strings.EqualFold(fields[1], "textLocale") || strings.EqualFold(fields[1], "audioLocale")) {
				continue
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, fmt.Sprintf("SET textLocale %q", locale), fmt.Sprintf("SET audioLocale %q", locale))
	updated := strings.Join(lines, newline) + newline
	if updated == string(data) {
		return nil
	}
	return casc.WriteFile(path, []byte(updated), mode)
}
