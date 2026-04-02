package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

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
