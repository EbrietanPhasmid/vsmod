package main

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

type ModDBResponse struct {
	Mod struct {
		Releases []struct {
			Modversion   string   `json:"modversion"`
			MainFile     string   `json:"mainfile"`
			GameVersions []string `json:"tags"`
		} `json:"releases"`
	} `json:"mod"`
}
