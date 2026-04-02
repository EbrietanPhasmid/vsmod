package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/vbauerster/mpb/v8"
)

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
