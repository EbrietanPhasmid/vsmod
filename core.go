package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type ModStats struct {
	sync.Mutex

	downloaded int

	cached int

	totalBytes int64
}

// Clears the modpack's directory of invalid files and creates it if empty.
func preparePackDir(dir string, mp Modpack) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create pack directory: %w", err)
	}
	// validFiles := make(map[string]bool)
	for _, m := range mp.Mods {
		// Note: We don't have the exact filename yet because that
		// comes from the Registry/API. However, we can approximate
		// or, better yet, we just clean up based on the Slug.
		// For now, let's look at the actual files in the directory.
		_ = m // Placeholder
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		found := false
		for _, m := range mp.Mods {
			if strings.HasPrefix(entry.Name(), m.Name) {
				found = true
				break
			}
		}

		if !found {
			target := filepath.Join(dir, entry.Name())
			if err := os.Remove(target); err != nil {
				return fmt.Errorf("failed to prune old mod %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

func DownloadModPack(mp Modpack) error {
	start := time.Now()
	reg := LoadRegistry()
	regChanged := false
	var regMu sync.Mutex

	packDir := filepath.Join(STORAGE_PATH, "packs", mp.Name)
	preparePackDir(packDir, mp)

	p := mpb.New(mpb.WithWidth(64))
	var wg sync.WaitGroup
	var stats ModStats
	var warnings []string
	var warnMu sync.Mutex
	errChan := make(chan error, len(mp.Mods))
	sem := make(chan struct{}, 4)

	fmt.Printf("%s%s▶ Installing modpack:%s %s\n\n", CBold, CCyan, CReset, mp.Name)

	for _, mod := range mp.Mods {
		wg.Add(1)
		sem <- struct{}{}
		go func(m Mod) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, wasFromApi, warn, err := resolveModMetadata(m, mp.GameVersion, reg, &regMu)
			if err != nil {
				errChan <- err
				return
			}
			if wasFromApi {
				regChanged = true
			}
			if warn != "" {
				warnMu.Lock()
				warnings = append(warnings, warn)
				warnMu.Unlock()
			}

			_, err = syncModToPack(m, meta, packDir, p, &stats)
			if err != nil {
				errChan <- err
				return
			}
		}(mod)
	}

	wg.Wait()
	p.Wait()
	if regChanged {
		reg.Save()
	}

	close(errChan)
	fmt.Println()
	for _, w := range warnings {
		fmt.Println(w)
	}

	if len(errChan) > 0 {
		return <-errChan
	}

	duration := time.Since(start).Round(time.Millisecond)
	printSummary(len(mp.Mods), stats.downloaded, stats.cached, stats.totalBytes, len(warnings), duration)

	return LinkModPack(packDir)
}

func resolveModMetadata(m Mod, targetGameVer string, reg Registry, mu *sync.Mutex) (*RegistryEntry, bool, string, error) {
	key := regKey(m.Name, m.Version)

	mu.Lock()
	entry, found := reg[key]
	mu.Unlock()

	if found {
		warn := checkCompatibility(m, targetGameVer, entry.GameVersions)
		if warn == "" {
			return &entry, false, warn, nil
		}

		shouldFetch := false

		for _, gv := range entry.GameVersions {
			if isMajorMismatch(gv, targetGameVer) {
				shouldFetch = true
			}

		}

		if shouldFetch {
			warn = fetchSuggestionLocally(m, targetGameVer, entry.GameVersions)
		}
		return &entry, false, warn, nil
	}

	resp, err := http.Get(fmt.Sprintf(BaseApiSlugUrl, m.Name))
	if err != nil {
		return nil, false, "", err
	}
	defer resp.Body.Close()

	var dbData ModDBResponse
	if err := json.NewDecoder(resp.Body).Decode(&dbData); err != nil {
		return nil, false, "", err
	}

	var target *ModRelease
	for i := range dbData.Mod.Releases {
		if dbData.Mod.Releases[i].Modversion == m.Version {
			target = &dbData.Mod.Releases[i]
			break
		}
	}

	if target == nil {
		return nil, false, "", fmt.Errorf("version %s not found for %s", m.Version, m.Name)
	}

	warn := checkCompatibility(m, targetGameVer, target.GameVersions)

	newEntry := RegistryEntry{
		ModSlug:      m.Name,
		Version:      m.Version,
		MainFile:     target.MainFile,
		GameVersions: target.GameVersions,
		LastUpdated:  time.Now(),
	}

	mu.Lock()
	reg[key] = newEntry
	mu.Unlock()

	return &newEntry, true, warn, nil
}

func syncModToPack(m Mod, meta *RegistryEntry, packDir string, p *mpb.Progress, stats *ModStats) (bool, error) {
	fileName := filepath.Base(meta.MainFile)
	cacheFile := filepath.Join(CACHE_PATH, fmt.Sprintf("%s_%s_%s", m.Name, m.Version, fileName))
	targetFile := filepath.Join(packDir, fileName)

	bar := p.AddBar(0,
		mpb.PrependDecorators(decor.Name(m.Name, decor.WC{W: 24, C: decor.DindentRight})),
		mpb.AppendDecorators(
			decor.OnComplete(
				decor.CountersKiloByte("% .2f / % .2f", decor.WC{W: 18}),
				fmt.Sprintf("%sOK%s", CGreen, CReset),
			),
		),
	)

	downloaded := false
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		if err := downloadFile(meta.MainFile, cacheFile, bar); err != nil {
			bar.Abort(false)
			return false, err
		}
		downloaded = true
	}

	fi, _ := os.Stat(cacheFile)
	bar.SetTotal(fi.Size(), true)

	stats.Lock()
	stats.totalBytes += fi.Size()
	if downloaded {
		stats.downloaded++
	} else {
		stats.cached++
	}
	stats.Unlock()

	os.Remove(targetFile)

	return downloaded, os.Link(cacheFile, targetFile)
}

func LinkModPack(packCachePath string) error {
	info, err := os.Lstat(MOD_PATH)
	if err == nil {
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			backupPath := MOD_PATH + ".backup"
			fmt.Printf("%s! Found existing Mods folder. Moving to %s%s\n", CYellow, backupPath, CReset)
			os.Rename(MOD_PATH, backupPath)
		} else {
			os.Remove(MOD_PATH)
		}
	}

	err = os.Symlink(packCachePath, MOD_PATH)
	if err != nil {
		return fmt.Errorf("failed to link modpack: %w", err)
	}

	return nil
}

func checkCompatibility(m Mod, targetGameVer string, modGameVersions []string) string {
	// If there are no tags, we assume it's a "universal" or legacy mod
	if len(modGameVersions) == 0 {
		return ""
	}

	// Check for exact or prefix match
	if slices.Contains(modGameVersions, targetGameVer) {
		return ""
	}

	// If we are here, there is a mismatch.
	warning := fmt.Sprintf("%s! Compatibility Warning: [%s]:%s Version %s is for %v (Pack is %s).",
		CYellow, m.Name, CReset, m.Version, modGameVersions, targetGameVer)

	return warning
}

func fetchSuggestionLocally(m Mod, targetGameVer string, currentModVersions []string) string {
	// API call to find a better version
	resp, err := http.Get(fmt.Sprintf(BaseApiSlugUrl, m.Name))
	if err != nil {
		return fmt.Sprintf("%s! Compatibility Warning: [%s] (Failed to fetch suggestions)%s", CYellow, m.Name, CReset)
	}
	defer resp.Body.Close()

	var dbData ModDBResponse
	json.NewDecoder(resp.Body).Decode(&dbData)

	var suggestion string
	for _, r := range dbData.Mod.Releases {
		if slices.Contains(r.GameVersions, targetGameVer) {
			suggestion = r.Modversion
			break
		}
	}

	warning := fmt.Sprintf("%s! Compatibility Warning: [%s]:%s Version %s is for %v (Pack is %s).",
		CYellow, m.Name, CReset, m.Version, currentModVersions, targetGameVer)

	if suggestion != "" {
		warning += fmt.Sprintf("\n\t%sSuggestion:%s Try version %s%s%s.", CCyan, CReset, CBold, suggestion, CReset)
	}

	return warning
}

func isMajorMismatch(v1, v2 string) bool {

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// If we can't parse them properly, assume it's a major change to be safe
	if len(parts1) < 2 || len(parts2) < 2 {
		return true
	}

	// Compare Major (index 0) and Minor (index 1)
	if parts1[0] != parts2[0] || parts1[1] != parts2[1] {
		return true
	}

	return false
}
