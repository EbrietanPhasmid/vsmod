package main

import (
	"encoding/json"
	"fmt"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

func DownloadModPack(mp Modpack) error {
	start := time.Now()
	fmt.Printf("%s%s▶ Attempting to install modpack:%s %s (%s)\n\n", CBold, CCyan, CReset, mp.Name, mp.GameVersion)

	var stats struct {
		sync.Mutex
		downloaded int
		cached     int
		totalBytes int64
	}

	packFolderName := fmt.Sprintf(mp.Name)
	packDir := filepath.Join(STORAGE_PATH, "packs", packFolderName)

	if err := CleanupOldMods(packDir, mp); err != nil {
		fmt.Printf("%s! Cleanup warning:%s %v\n", CYellow, err, CReset)
	}
	os.MkdirAll(packDir, 0755)

	p := mpb.New(mpb.WithWidth(64), mpb.WithRefreshRate(100*time.Millisecond))
	var wg sync.WaitGroup
	var warning_messages []string
	var warnings = 0
	var mu sync.Mutex
	errChan := make(chan error, len(mp.Mods))

	sem := make(chan struct{}, 4)

	for _, mod := range mp.Mods {
		wg.Add(1)
		sem <- struct{}{}

		go func(m Mod) {
			defer wg.Done()
			defer func() { <-sem }()

			resp, err := http.Get(fmt.Sprintf(BaseApiSlugUrl, m.Name))
			if err != nil {
				errChan <- fmt.Errorf("api request failed for %s: %w", m.Name, err)
				return
			}
			defer resp.Body.Close()

			var dbData ModDBResponse
			if err := json.NewDecoder(resp.Body).Decode(&dbData); err != nil {
				errChan <- fmt.Errorf("failed to decode api response for %s: %w", m.Name, err)
				return
			}

			var targetRelease *struct {
				Modversion   string   `json:"modversion"`
				MainFile     string   `json:"mainfile"`
				GameVersions []string `json:"tags"`
			}

			for i := range dbData.Mod.Releases {
				if dbData.Mod.Releases[i].Modversion == m.Version {
					targetRelease = &dbData.Mod.Releases[i]
					break
				}
			}

			if targetRelease != nil {
				isCompatible := false

				// empty compatibility list is vacuously compatible
				if len(targetRelease.GameVersions) < 1 {
					isCompatible = true
				}

				if !isCompatible {
					if slices.Contains(targetRelease.GameVersions, mp.GameVersion) {
						isCompatible = true
					}
				}

				if !isCompatible {
					var suggestion = ""
					for _, r := range dbData.Mod.Releases {
						if slices.Contains(r.GameVersions, mp.GameVersion) {
							suggestion = r.Modversion
							break
						}
					}
					msg := fmt.Sprintf("\n%s! Compatibility Warning: [%s]:%s\nVersion %s is for %v.\nYour pack is %s.\n", CYellow, m.Name, CReset, m.Version, targetRelease.GameVersions, mp.GameVersion)
					if suggestion != "" {
						msg += fmt.Sprintf("    %sSuggestion:%s Try using version %s%s%s instead.\n", CCyan, CReset, CBold, suggestion, CReset)
					}
					mu.Lock()
					warnings++
					warning_messages = append(warning_messages, msg)
					mu.Unlock()
				}

			}

			var downloadPath string
			for _, release := range dbData.Mod.Releases {
				if release.Modversion == m.Version {
					downloadPath = release.MainFile
					break
				}
			}

			if downloadPath == "" {
				errChan <- fmt.Errorf("version mismatch: %s was provided with version '%s', but API only has versions like '%s'",
					m.Name, m.Version, dbData.Mod.Releases[0].Modversion)
				return
			}

			fileName := filepath.Base(downloadPath)
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

			if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
				if err := downloadFile(downloadPath, cacheFile, bar); err != nil {
					bar.Abort(false)
					errChan <- err
					return
				}
				fi, _ := os.Stat(cacheFile)
				stats.Lock()
				stats.downloaded++
				stats.totalBytes += fi.Size()
				stats.Unlock()
			} else {
				fi, _ := os.Stat(cacheFile)
				bar.SetTotal(fi.Size(), true)
				stats.Lock()
				stats.cached++
				stats.totalBytes += fi.Size()
				stats.Unlock()
			}

			if err := copyFile(cacheFile, targetFile); err != nil {
				errChan <- fmt.Errorf("failed to sync from cache: %w", err)
				bar.Abort(false)
				return
			}

			fi, _ := os.Stat(cacheFile)
			bar.SetTotal(fi.Size(), true)
			copyFile(cacheFile, targetFile)
		}(mod)
	}

	wg.Wait()
	p.Wait()

	close(errChan)
	if len(errChan) > 0 {
		return <-errChan
	}

	if len(warning_messages) > 0 {
		for _, w := range warning_messages {
			fmt.Printf("%s\n", w)
		}
		fmt.Println()
	}

	duration := time.Since(start).Round(time.Millisecond)
	printSummary(len(mp.Mods), stats.downloaded, stats.cached, stats.totalBytes, warnings, duration)

	fmt.Printf("\n%s✔ Download complete!%s\n", CGreen, CReset)
	return LinkModPack(packDir)
}

func LinkModPack(packCachePath string) error {
	info, err := os.Lstat(MOD_PATH)
	if err == nil {
		// If it's a real directory back it up
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			backupPath := MOD_PATH + ".backup"
			fmt.Printf("%s! Found existing Mods folder. Moving to %s%s\n", CYellow, backupPath, CReset)
			os.Rename(MOD_PATH, backupPath)
		} else {
			// If it's a symlink remove the link
			os.Remove(MOD_PATH)
		}
	}

	// 2. Create the symlink: Game/Mods -> Cache/PackName
	err = os.Symlink(packCachePath, MOD_PATH)
	if err != nil {
		return fmt.Errorf("failed to link modpack: %w", err)
	}

	return nil
}
