package main

import (
	"time"
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

type ModRelease struct {
	ReleaseID    int      `json:"releaseid"`
	Modversion   string   `json:"modversion"`
	MainFile     string   `json:"mainfile"`
	Filename     string   `json:"filename"`
	Downloads    int      `json:"downloads"`
	GameVersions []string `json:"tags"`
	Created      string   `json:"created"`
}

type ModDBResponse struct {
	Status string `json:"status"`
	Mod    struct {
		Name     string       `json:"name"`
		Author   string       `json:"author"`
		Releases []ModRelease `json:"releases"`
	} `json:"mod"`
}

type Registry map[string]RegistryEntry

type RegistryEntry struct {
	ModSlug      string    `json:"slug"`
	Version      string    `json:"version"`
	MainFile     string    `json:"main_file"`
	GameVersions []string  `json:"game_versions"`
	LastUpdated  time.Time `json:"last_updated"`
}
