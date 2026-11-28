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
	"strconv"
	"strings"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/cdn"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/install"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/pin"
)

var appVersion = "v1.9.4"

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
	err := run(ctx, input, args, "enUS", func(format string, args ...any) { fmt.Printf(format+"\n", args...) }, nil)
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

// highResFlag adds the optional high-resolution textures (the download manifest's HighRes content, about 37 GiB),
// which are left out by default.
const highResFlag = "--high-res"

func run(ctx context.Context, input *bufio.Reader, args []string, locale string, logf func(string, ...any), dataProgress func(int, int)) error {
	target := pin.Default()
	var optional []string
	if len(args) > 0 && args[0] == highResFlag {
		optional, args = []string{"HighRes"}, args[1:]
	}
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
		for _, prompt := range []string{"Carpeta de World of Warcraft: ", "Carpeta separada para la " + version + ": "} {
			fmt.Print(prompt)
			line, err := input.ReadString('\n')
			if err != nil {
				return err
			}
			args = append(args, strings.Trim(strings.TrimSpace(line), "\""))
		}
	}
	if len(args) != 2 || args[0] == "" || args[1] == "" {
		return errors.New("uso: Molten-Core-WoW-Forever-Downgrader.exe [--high-res] [--target <version>] <carpeta WoW Forever> <carpeta destino>")
	}
	source := filepath.Clean(args[0])
	if strings.EqualFold(filepath.Base(source), "_forever_") {
		source = filepath.Dir(source)
	}
	rows, err := install.ActiveBuilds(source)
	if err != nil {
		return fmt.Errorf("leer version de origen: %w", err)
	}
	sourceVersion := ""
	for _, row := range rows {
		if row.Product == "wowf" && versionPattern.MatchString(row.Version) {
			sourceVersion = row.Version
		}
	}
	if sourceVersion == "" {
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
	// The source is the up-to-date Battle.net installation; it is only read.
	logf("Origen / Source (Battle.net): %s -> destino / target: %s", sourceVersion, target.Version)
	if compareVersions(target.Version, sourceVersion) >= 0 {
		logf("Aviso / Warning: %s no es anterior a la version de Battle.net / is not older than the Battle.net version", target.Version)
	}
	hosts := target.Hosts(ctx, nil)
	if err := ctx.Err(); err != nil {
		return err
	}
	// A complete client: every data file the download manifest lists for this
	// platform and language (plus the high-resolution textures if asked), not only
	// the startup set the game would otherwise stream later. Matching files are copied from the source installation and
	// only the rest is fetched, in exact byte ranges.
	report, err := install.Run(ctx, install.Options{
		Dir: destination, ReuseFrom: source, Pin: target,
		CDN: cdn.New(append([]string{manifestHost}, hosts...), target.CDNPath), GameHosts: hosts,
		Tags: clientTags(locale),
		// Optional content (high-resolution textures) only when asked for.
		OptionalContent: optional,
		ExactRanges:     true,
		Log:             logf,
		DataProgress:    dataProgress,
	})
	if err != nil {
		return err
	}
	if report.Failed() {
		return errors.New("la instalacion contiene archivos faltantes o invalidos")
	}
	if err := configureGameLocale(filepath.Join(destination, target.GameFolder), locale); err != nil {
		return fmt.Errorf("configurar idioma del juego / configure game language: %w", err)
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

func clientTags(locale string) []string {
	platform, arch := "Windows", "x86_64"
	if runtime.GOOS == "darwin" {
		platform = "OSX"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
	}
	return []string{platform, arch, locale, "US"}
}

// compareVersions orders two four-part versions (such as 1.60.1.71037)
// numerically: -1, 0 or 1.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
