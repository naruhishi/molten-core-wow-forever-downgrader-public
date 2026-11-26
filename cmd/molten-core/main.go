package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/cdn"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/install"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/pin"
)

var appVersion = "v1.8.1"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--app-version" {
		fmt.Println(appVersion)
		return
	}
	if len(os.Args) == 1 && launchGUI() {
		return
	}
	input := bufio.NewReader(os.Stdin)
	interactive := len(os.Args) == 1
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	args := os.Args[1:]
	if len(args) > 1 && args[0] == "--cancel-on-stdin-close" {
		args = args[1:]
		parentCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		ctx = parentCtx
		go func() {
			_, _ = io.Copy(io.Discard, input)
			cancel()
		}()
	}
	err := run(ctx, input, args, func(format string, args ...any) { fmt.Printf(format+"\n", args...) })
	if err != nil && ctx.Err() != nil {
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	if interactive {
		fmt.Print("Pulsa Enter para cerrar.")
		input.ReadString('\n')
	}
	if err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, input *bufio.Reader, args []string, logf func(string, ...any)) error {
	target := pin.Default()
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(target.Version)
		return nil
	}
	if len(args) == 2 && args[0] == "--validate" {
		_, err := validate(ctx, args[1], logf)
		return err
	}
	version := target.Version
	if len(args) == 4 && args[0] == "--target" {
		version, args = args[1], args[2:]
	}
	if len(args) == 0 {
		fmt.Println("Molten Core - WoW Forever Downgrader")
		fmt.Println("Cierra WoW y Battle.net antes de continuar.")
		for _, prompt := range []string{"Carpeta de World of Warcraft: ", "Carpeta separada para la 71037: "} {
			fmt.Print(prompt)
			line, err := input.ReadString('\n')
			if err != nil {
				return err
			}
			args = append(args, strings.Trim(strings.TrimSpace(line), "\""))
		}
	}
	if len(args) != 2 || args[0] == "" || args[1] == "" {
		return errors.New("uso: Molten-Core-WoW-Forever-Downgrader.exe <carpeta WoW Forever> <carpeta destino>")
	}
	source := filepath.Clean(args[0])
	if strings.EqualFold(filepath.Base(source), "_forever_") {
		source = filepath.Dir(source)
	}
	rows, err := install.ActiveBuilds(source)
	if err != nil {
		return fmt.Errorf("leer version de origen: %w", err)
	}
	found := false
	for _, row := range rows {
		if row.Product == "wowf" && versionPattern.MatchString(row.Version) {
			found = true
		}
	}
	if !found {
		return errors.New("el origen no tiene una instalacion activa de WoW Forever / no active WoW Forever installation")
	}
	destination, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("el destino debe ser una carpeta normal")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("la carpeta que contiene el destino debe existir: %w", err)
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	if _, err := os.Stat(destination); err == nil {
		destination, err = filepath.EvalSymlinks(destination)
		if err != nil {
			return err
		}
	}
	for _, paths := range [][2]string{{source, destination}, {destination, source}} {
		rel, err := filepath.Rel(paths[0], paths[1])
		if err == nil && filepath.IsLocal(rel) {
			return errors.New("origen y destino deben ser carpetas separadas")
		}
	}
	if info != nil {
		entries, err := os.ReadDir(destination)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			switch entry.Name() {
			case ".everlake", ".build.info", "_forever_", "Data":
			default:
				return errors.New("elige una carpeta vacia o la misma carpeta de un intento anterior")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err = validate(ctx, version, logf)
	if err != nil {
		return err
	}
	hosts := target.Hosts(ctx, nil)
	if err := ctx.Err(); err != nil {
		return err
	}
	report, err := install.Run(ctx, install.Options{
		Dir: destination, ReuseFrom: source, Pin: target,
		CDN: cdn.New(append([]string{manifestHost}, hosts...), target.CDNPath), GameHosts: hosts,
		Tags: clientTags(),
		Log:  logf,
	})
	if err != nil {
		return err
	}
	if report.Failed() {
		return errors.New("la instalacion contiene archivos faltantes o invalidos")
	}
	logf("Listo / Ready. %.2f GiB. Cliente / Client: %s", float64(report.BytesDownloaded)/(1<<30), filepath.Join(destination, target.GameFolder))
	return nil
}

func validate(ctx context.Context, version string, logf func(string, ...any)) (pin.Pin, error) {
	logf("Validando / Checking WoW Forever %s...", version)
	target, err := resolveTarget(ctx, version)
	if err == nil {
		logf("%s: manifiestos disponibles / manifests available", target.Version)
	}
	return target, err
}

func clientTags() []string {
	platform, arch := "Windows", "x86_64"
	if runtime.GOOS == "darwin" {
		platform = "OSX"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
	}
	return []string{platform, arch, "enUS", "US"}
}
