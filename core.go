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

type InstallStatus struct {
	sync.Mutex
	Downloaded int
	Cached     int
	TotalBytes int64
	Warnings   []string
}

// PackInstaller handles the orchestration of the installation lifecycle.
type PackInstaller struct {
	Pack       Modpack
	Registry   Registry
	Progress   *mpb.Progress
	Stats      *InstallStatus
	PackDir    string
	regMu      sync.Mutex
	regChanged bool
}

// ModTask represents a single unit of work in the pipeline.
type ModTask struct {
	Source   Mod
	Meta     *RegistryEntry
	Warning  string
	IsRemote bool
}

func NewPackInstaller(mp Modpack) *PackInstaller {
	return &PackInstaller{
		Pack:     mp,
		Registry: LoadRegistry(),
		Progress: mpb.New(mpb.WithWidth(64)),
		Stats:    &InstallStatus{},
		PackDir:  filepath.Join(STORAGE_PATH, "packs", mp.Name),
	}
}

func (i *PackInstaller) Install() error {
	start := time.Now()

	// 1. Prepare Filesystem
	if err := i.prepareFilesystem(); err != nil {
		return err
	}

	fmt.Printf("%s%s▶ Installing modpack:%s %s\n\n", CBold, CCyan, CReset, i.Pack.Name)

	// 2. Parallel Pipeline Execution
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	errChan := make(chan error, len(i.Pack.Mods))

	for _, m := range i.Pack.Mods {
		wg.Add(1)
		go func(mod Mod) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := i.processMod(mod); err != nil {
				errChan <- err
			}
		}(m)
	}

	wg.Wait()
	i.Progress.Wait()

	// 3. Post-Process State
	if i.regChanged {
		i.Registry.Save()
	}

	close(errChan)
	i.printOutput(start)

	if len(errChan) > 0 {
		return <-errChan
	}

	return LinkModPack(i.PackDir)
}

func (i *PackInstaller) processMod(m Mod) error {
	// State 1: Resolve Metadata
	task, err := i.resolve(m)
	if err != nil {
		return err
	}

	if task.Warning != "" {
		i.Stats.Lock()
		i.Stats.Warnings = append(i.Stats.Warnings, task.Warning)
		i.Stats.Unlock()
	}

	// State 2: Sync Filesystem
	return i.sync(task)
}

func (i *PackInstaller) resolve(m Mod) (*ModTask, error) {
	key := regKey(m.Name, m.Version)
	task := &ModTask{Source: m}

	i.regMu.Lock()
	entry, found := i.Registry[key]
	i.regMu.Unlock()

	if found {
		task.Meta = &entry
		task.Warning = i.checkCompatibility(m, entry.GameVersions, nil)

		// Lazy-fetch suggestion if major mismatch
		if task.Warning != "" {
			for _, gv := range entry.GameVersions {
				if isMajorMismatch(gv, i.Pack.GameVersion) {
					task.Warning = i.fetchSuggestionLocally(m, entry.GameVersions)
					break
				}
			}
		}
		return task, nil
	}

	// Cold Path: Fetch from API
	resp, err := http.Get(fmt.Sprintf(BaseApiSlugUrl, m.Name))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var db ModDBResponse
	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		return nil, err
	}

	var release *ModRelease
	for idx := range db.Mod.Releases {
		if db.Mod.Releases[idx].Modversion == m.Version {
			release = &db.Mod.Releases[idx]
			break
		}
	}

	if release == nil {
		return nil, fmt.Errorf("version %s not found for %s", m.Version, m.Name)
	}

	task.Meta = &RegistryEntry{
		ModSlug:      m.Name,
		Version:      m.Version,
		MainFile:     release.MainFile,
		GameVersions: release.GameVersions,
		LastUpdated:  time.Now(),
	}
	task.IsRemote = true
	task.Warning = i.checkCompatibility(m, release.GameVersions, db.Mod.Releases)

	i.regMu.Lock()
	i.Registry[key] = *task.Meta
	i.regChanged = true
	i.regMu.Unlock()

	return task, nil
}

func (i *PackInstaller) sync(t *ModTask) error {
	fileName := filepath.Base(t.Meta.MainFile)
	cachePath := filepath.Join(CACHE_PATH, fmt.Sprintf("%s_%s_%s", t.Source.Name, t.Source.Version, fileName))
	targetPath := filepath.Join(i.PackDir, fileName)

	bar := i.Progress.AddBar(0,
		mpb.PrependDecorators(decor.Name(t.Source.Name, decor.WC{W: 24, C: decor.DindentRight})),
		mpb.AppendDecorators(decor.OnComplete(decor.CountersKiloByte("% .2f / % .2f", decor.WC{W: 18}), fmt.Sprintf("%sDONE%s", CGreen, CReset))),
	)

	isNewDownload := false
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		if err := downloadFile(t.Meta.MainFile, cachePath, bar); err != nil {
			bar.Abort(false)
			return err
		}
		isNewDownload = true
	}

	info, _ := os.Stat(cachePath)
	bar.SetTotal(info.Size(), true)

	i.Stats.Lock()
	i.Stats.TotalBytes += info.Size()
	if isNewDownload {
		i.Stats.Downloaded++
	} else {
		i.Stats.Cached++
	}
	i.Stats.Unlock()

	os.Remove(targetPath)
	return os.Link(cachePath, targetPath)
}

func (i *PackInstaller) prepareFilesystem() error {
	if err := os.MkdirAll(i.PackDir, 0755); err != nil {
		return err
	}
	entries, _ := os.ReadDir(i.PackDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := false
		for _, m := range i.Pack.Mods {
			if strings.HasPrefix(entry.Name(), m.Name) {
				match = true
				break
			}
		}
		if !match {
			os.Remove(filepath.Join(i.PackDir, entry.Name()))
		}
	}
	return nil
}

func (i *PackInstaller) printOutput(start time.Time) {
	fmt.Println()
	for _, w := range i.Stats.Warnings {
		fmt.Println(w)
	}
	duration := time.Since(start).Round(time.Millisecond)
	printSummary(len(i.Pack.Mods), i.Stats.Downloaded, i.Stats.Cached, i.Stats.TotalBytes, len(i.Stats.Warnings), duration)
}

// Standard logic remain separate as "Pure Functions"
func (i *PackInstaller) checkCompatibility(m Mod, versions []string, releases []ModRelease) string {
	if len(versions) == 0 || slices.Contains(versions, i.Pack.GameVersion) {
		return ""
	}
	warn := fmt.Sprintf("%s! Compatibility Warning: [%s]:%s Version %s is for %v (Pack is %s).",
		CYellow, m.Name, CReset, m.Version, versions, i.Pack.GameVersion)

	if len(releases) > 0 {
		for _, r := range releases {
			if slices.Contains(r.GameVersions, i.Pack.GameVersion) {
				warn += fmt.Sprintf("\n\t%sSuggestion:%s Try version %s%s%s.", CCyan, CReset, CBold, r.Modversion, CReset)
				break
			}
		}
	}
	return warn
}

func (i *PackInstaller) fetchSuggestionLocally(m Mod, versions []string) string {
	resp, _ := http.Get(fmt.Sprintf(BaseApiSlugUrl, m.Name))
	if resp == nil {
		return ""
	}
	defer resp.Body.Close()
	var db ModDBResponse
	json.NewDecoder(resp.Body).Decode(&db)
	return i.checkCompatibility(m, versions, db.Mod.Releases)
}

func isMajorMismatch(v1, v2 string) bool {
	p1, p2 := strings.Split(v1, "."), strings.Split(v2, ".")
	if len(p1) < 2 || len(p2) < 2 {
		return true
	}
	return p1[0] != p2[0] || p1[1] != p2[1]
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
