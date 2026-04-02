package main

// TODO
// - conservative cleanup
// - split into smaller files

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

var (
	VERSION      = "dev-build"
	STORAGE_PATH = ""
	CACHE_PATH   = ""
	MOD_PATH     = ""
)

const (
	BaseApiSlugUrl = "https://mods.vintagestory.at/api/mod/%s"
)

const (
	CReset  = "\033[0m"
	CBold   = "\033[1m"
	CGreen  = "\033[32m"
	CYellow = "\033[33m"
	CBlue   = "\033[34m"
	CCyan   = "\033[36m"
	CRed    = "\033[31m"
)

type (
	Modpack struct {
		Name        string `toml:"name"`
		GameVersion string `toml:"game_version"`
		Mods        []Mod  `toml:"mods"`
	}
	Mod struct {
		Name    string `toml:"name"`
		Version string `toml:"version"`
	}
)

func init() {
	if err := setupPaths(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error setting up paths: %v\n", err)
		os.Exit(1)
	}
}

func setupPaths() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	STORAGE_PATH = filepath.Join(home, ".vsmod")
	if err := os.MkdirAll(STORAGE_PATH, 0755); err != nil {
		return err
	}

	CACHE_PATH = filepath.Join(STORAGE_PATH, "cache")
	if err := os.MkdirAll(CACHE_PATH, 0755); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "windows":
		STORAGE_PATH := os.Getenv("APPDATA")
		if STORAGE_PATH == "" {
			return fmt.Errorf("APPDATA not set")
		}
		MOD_PATH = filepath.Join(STORAGE_PATH, "VintagestoryData", "Mods")
	case "linux":
		MOD_PATH = filepath.Join(home, ".config", "VintagestoryData", "Mods")
	case "darwin":
		MOD_PATH = filepath.Join(home, "Library", "Application Support", "VintagestoryData", "Mods")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return nil
}

func Parse(filePath string) (*Modpack, error) {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("modpack file not found: %s", filePath)
	}

	blob, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var pack Modpack
	if _, err := toml.Decode(string(blob), &pack); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if pack.Name == "" {
		return nil, fmt.Errorf("invalid modpack: 'name' is required")
	}
	if len(pack.Mods) == 0 {
		return nil, fmt.Errorf("invalid modpack: at least one mod must be listed")
	}

	return &pack, nil
}

type ModDBResponse struct {
	Mod struct {
		Releases []struct {
			Modversion   string   `json:"modversion"`
			MainFile     string   `json:"mainfile"`
			GameVersions []string `json:"tags"`
		} `json:"releases"`
	} `json:"mod"`
}

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

func downloadFile(rawUrl string, dest string, bar *mpb.Bar) error {
	url, err := url.Parse(rawUrl)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	url.RawQuery = url.Query().Encode()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{}
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}

	// The User-Agent keeps the CDN happy
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) vsmmd-manager/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bar.SetTotal(resp.ContentLength, false)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s (Check if the link is expired or blocked)", resp.Status)
	}

	proxyReader := bar.ProxyReader(resp.Body)

	_, err = io.Copy(out, proxyReader)
	return err
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

// TODO : make `CleanupOldMods` retain functional files present in the modpack.
// have it only eliminate redundant files.
func CleanupOldMods(packDir string, mp Modpack) error {
	// files, err := os.ReadDir(packDir)
	// if err != nil {
	// 	return err
	// }

	// allowed := make(map[string]bool)
	// for _, m := range mp.Mods {
	//  ......
	// }

	return os.RemoveAll(packDir)
}

func SetDefaultModPath() error {

	switch runtime.GOOS {
	case "windows":
		// Fetch %AppData%
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return fmt.Errorf("APPDATA environment variable not set")
		}
		MOD_PATH = filepath.Join(appData, "VintagestoryData", "Mods")

	case "linux":
		// Usually ~/.config/VintagestoryData/Mods
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		MOD_PATH = filepath.Join(home, ".config", "VintagestoryData", "Mods")

	case "darwin": // macOS
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		MOD_PATH = filepath.Join(home, "Library", "Application Support", "VintagestoryData", "Mods")

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func main() {

	var rootCmd = &cobra.Command{
		Use:   "vsmod",
		Short: "Vintage Story Mod Manager",
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version of the currently installed vsmod binary",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(VERSION)
		},
	}

	var installCmd = &cobra.Command{
		Use:   "install [toml file]",
		Short: "Install a modpack from a TOML file",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			pack, err := Parse(path)
			if err != nil {
				fmt.Printf("%sError: %v%s\n", CRed, err, CReset)
				return
			}
			if err := DownloadModPack(*pack); err != nil {
				fmt.Printf("%sError: %v%s\n", CRed, err, CReset)
			}
		},
	}

	var clearCmd = &cobra.Command{
		Use:   "clear [data]",
		Short: "Clear VSMod's data",
	}

	var clearCacheCmd = &cobra.Command{
		Use:   "cache",
		Short: "Clears VSMod's cache",
		Run: func(cmd *cobra.Command, args []string) {
			os.RemoveAll(CACHE_PATH)
		},
	}
	var linkCmd = &cobra.Command{
		Use:   "link [modpack name]",
		Short: "Sets the modpack as currently in use",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			packname := args[0]
			packpath := filepath.Join(STORAGE_PATH, "packs", packname)

			fi, err := os.Stat(packpath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("%sError: Modpack '%s' not found in %s%s\n", CRed, packname, packpath, CReset)
				} else {
					fmt.Printf("%sError: %v%s\n", CRed, err, CReset)
				}
				return
			}

			if !fi.IsDir() {
				fmt.Printf("%sError: %s is not a directory%s\n", CRed, packpath, CReset)
				return
			}

			if err := LinkModPack(packpath); err != nil {
				fmt.Printf("%sError linking pack: %v%s\n", CRed, err, CReset)
				return
			}

			fmt.Printf("%s✔ Successfully linked modpack: %s%s\n", CGreen, packname, CReset)
		},
	}

	var linkListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all installed modpacks",
		Run: func(cmd *cobra.Command, args []string) {
			packsDir := filepath.Join(STORAGE_PATH, "packs")

			entries, err := os.ReadDir(packsDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("%sNo modpacks found. Install one first!%s\n", CYellow, CReset)
					return
				}
				fmt.Printf("%sError reading packs: %v%s\n", CRed, err, CReset)
				return
			}

			activePack := ""
			currentLink, _ := os.Readlink(MOD_PATH)
			if err == nil {
				activePack = filepath.Base((currentLink))
			}

			fmt.Printf("\n%s%sAvailable Modpacks:%s\n", CBold, CCyan, CReset)
			fmt.Println("------------------")

			found := false
			for _, entry := range entries {
				if entry.IsDir() {
					var symbol = "•"
					if activePack == entry.Name() {
						symbol = "+"
					}
					fmt.Printf("  %s%s%s %s\n", CCyan, symbol, CReset, entry.Name())
					found = true
				}
			}

			if !found {
				fmt.Println("  (No modpack directories found)")
			}
			fmt.Println()
		},
	}

	linkCmd.AddCommand(linkListCmd)
	clearCmd.AddCommand(clearCacheCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(clearCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.Execute()
}

func printSummary(total, dl, cached int, size int64, warnings int, duration time.Duration) {
	fmt.Println(CBold + "Installation Summary" + CReset)
	fmt.Println("---------------------------------")
	fmt.Printf(" Total Mods:     %d\n", total)
	fmt.Printf(" Downloaded:     %s%d%s\n", CGreen, dl, CReset)
	fmt.Printf(" From Cache:     %s%d%s\n", CYellow, cached, CReset)
	fmt.Printf(" Total Size:     %.2f MB\n", float64(size)/(1024*1024))
	fmt.Printf(" Warnings:       %s%d%s\n", CYellow, warnings, CReset)
	fmt.Printf(" Time Elapsed:   %v\n", duration)
	fmt.Println("---------------------------------")
}
