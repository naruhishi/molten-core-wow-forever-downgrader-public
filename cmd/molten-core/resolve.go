package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/buildthehomelab/wow-everlake-launcher/internal/cdn"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/pin"
	"github.com/buildthehomelab/wow-everlake-launcher/internal/tact"
)

const catalog = "https://blizztrack.com/api/manifest/wowf/"
const manifestHost = "https://us.cdn.blizzard.com"

var versionPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[1-9][0-9]{0,8}$`)
var lookupHTTP = &http.Client{Timeout: 7 * time.Second}

type versionRow struct {
	Version string `json:"version_name"`
	ID      int    `json:"build_id"`
	Build   string `json:"build_config"`
	CDN     string `json:"cdn_config"`
}

func lookupJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalog+endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := lookupHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("BlizzTrack HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return fmt.Errorf("BlizzTrack no devolvio datos")
	}
	return json.Unmarshal(envelope.Result, out)
}

func readVersions(ctx context.Context, suffix string) ([]versionRow, error) {
	var result struct {
		Data []versionRow `json:"data"`
	}
	err := lookupJSON(ctx, "versions"+suffix, &result)
	return result.Data, err
}

func resolveTarget(ctx context.Context, version string) (pin.Pin, error) {
	base := pin.Default()
	if !versionPattern.MatchString(version) {
		return base, fmt.Errorf("escribe la version completa, por ejemplo 1.60.1.71037 / enter the full version")
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	try := func(rows []versionRow) (pin.Pin, bool, error) {
		for _, row := range rows {
			if row.Version != version {
				continue
			}
			p := base
			p.Version, p.BuildID, p.BuildConfig, p.CDNConfig = row.Version, row.ID, row.Build, row.CDN
			return p, true, verifyTarget(ctx, p)
		}
		return base, false, nil
	}
	current, err := readVersions(ctx, "")
	if err == nil {
		if p, found, err := try(current); found {
			return p, err
		}
	}
	if version == base.Version {
		return base, verifyTarget(ctx, base)
	}
	if err != nil {
		return base, fmt.Errorf("no se pudo consultar el catalogo / catalog unavailable: %w", err)
	}
	for page := 1; page <= 2; page++ {
		var history struct {
			Results []struct {
				Seqn int `json:"seqn"`
			} `json:"results"`
			TotalPages int `json:"total_pages"`
		}
		err := lookupJSON(ctx, "seqn?file=versions&page="+strconv.Itoa(page), &history)
		if err != nil {
			return base, err
		}
		for _, entry := range history.Results {
			if entry.Seqn <= 0 {
				continue
			}
			rows, err := readVersions(ctx, "?seqn="+strconv.Itoa(entry.Seqn))
			if err != nil {
				return base, fmt.Errorf("historial incompleto / history unavailable: %w", err)
			}
			if p, found, err := try(rows); found {
				return p, err
			}
		}
		if page >= history.TotalPages || len(history.Results) == 0 {
			break
		}
	}
	return base, fmt.Errorf("version %s no encontrada en el historial reciente de WoW Forever / not found in recent history", version)
}

func verifyTarget(ctx context.Context, p pin.Pin) error {
	if err := p.Check(); err != nil {
		return err
	}
	client := cdn.New([]string{manifestHost}, p.CDNPath)
	client.HTTP, client.Tries = lookupHTTP, 1
	getConfig := func(hash string) ([]byte, error) {
		key, err := tact.ParseKey(hash)
		if err != nil {
			return nil, err
		}
		return client.Do(ctx, cdn.Request{Kind: "config", Key: key, Max: 8 << 20,
			Check: func(data []byte) error {
				if md5.Sum(data) != key {
					return fmt.Errorf("CDN config hash mismatch")
				}
				return nil
			}})
	}
	build, err := getConfig(p.BuildConfig)
	if err != nil {
		return fmt.Errorf("configuracion de build no disponible / build config unavailable: %w", err)
	}
	cfg := tact.ParseConfig(string(build))
	name := cfg.One("build-name")
	prefix := fmt.Sprintf("WOW-%dpatch%s", p.BuildID, p.Version[:strings.LastIndexByte(p.Version, '.')])
	if cfg.One("build-uid") != "wowf" || (name != prefix && !strings.HasPrefix(name, prefix+"_")) {
		return fmt.Errorf("el manifiesto no coincide con WoW Forever %s / build manifest mismatch", p.Version)
	}
	if _, err := getConfig(p.CDNConfig); err != nil {
		return fmt.Errorf("CDN config unavailable: %w", err)
	}
	for _, manifest := range []string{"install", "download", "encoding"} {
		_, encoded, err := cfg.KeyPair(manifest)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, client.URL(manifestHost, "data", encoded, ""), nil)
		if err != nil {
			return err
		}
		resp, err := lookupHTTP.Do(req)
		if err != nil {
			return fmt.Errorf("manifest %s: %w", manifest, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.ContentLength == 0 {
			return fmt.Errorf("manifest %s unavailable (HTTP %d)", manifest, resp.StatusCode)
		}
	}
	return nil
}
