![vsmod downloading a modpack...](https://github.com/EbrietanPhasmid/vsmod/blob/master/vsmod.gif)
# vsmod 
`vsmod` is a CLI-based mod management utility for Vintage Story, developed in Go. It provides deterministic modpack environments through a global content-addressable storage (CAS) model and filesystem abstraction via symbolic links.

## 1. Architectural Overview

### 1.1 Filesystem Hierarchy
The tool manages state within a dedicated storage directory (default: `~/.vsmod`).

* **Global Cache (`~/.vsmod/cache/`)**: A persistent repository for unique mod binaries. Naming convention: `{mod_slug}_{version}_{original_filename}`.
* **Pack Storage (`~/.vsmod/packs/`)**: Contains discrete, isolated directories for each modpack. Each pack directory is a "resolved" state of a TOML manifest.
* **Game Integration (Symlinking)**: `vsmod` takes ownership of the `VintagestoryData/Mods` path. 
    * **First Run**: If a physical `Mods` directory exists, it is moved to `Mods.backup`.
    * **Linking**: `VintagestoryData/Mods` is created as a symbolic link pointing to a specific child directory in `~/.vsmod/packs/`. This allows for near-instantaneous switching between modpacks without moving bulk data.

### 1.2 Execution Pipeline
1.  **Manifest Parsing**: Decodes TOML and validates schema requirements.
2.  **API Resolution**: Queries `https://mods.vintagestory.at/api/mod/{slug}` to retrieve release metadata and verify version availability.
3.  **Compatibility Analysis**: Performs prefix-based version matching (e.g., Game Version `1.21` satisfies a mod tagged for `1.21.6`).
4.  **Concurrent Retrieval**: Utilizes a worker pool (semaphore-gated) to download assets into the Global Cache.
5.  **Environment Mutation**: Populates the local pack directory from the cache and performs an atomic swap of the `Mods` symlink.

---

## 2. Command Reference

### `install [path/to/manifest.toml]`
Syncs the specified manifest to the local filesystem.
* **Sync Logic**: Checks the global cache via `os.Stat`. Only missing assets are requested from the API.
* **IO Operations**: Files are copied/linked from the global cache to the specific pack directory to ensure environment isolation.

### `link [modpack_name]`
Updates the `VintagestoryData/Mods` symlink to point to an existing directory in `~/.vsmod/packs/`. 
* **Validation**: Errors gracefully if the requested pack has not been installed.

### `link list`
Queries `~/.vsmod/packs/` and identifies the currently active pack.
* **State Detection**: Uses `os.Readlink` on the game's `Mods` path to determine the "Source of Truth" for the active environment.
* **UI**: Active packs are denoted with a `+` marker and highlighted in green.

### `clear cache`
Recursively removes all files within `~/.vsmod/cache/`. This is a destructive operation used to reclaim disk space.

---

## 3. Manifest Schema (TOML)

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | String | Unique identifier for the modpack (used for folder naming). |
| `game_version` | String | Target Vintage Story version (used for compatibility logic). |
| `[[mods]]` | Array | A collection of mod objects. |
| `mods.name` | String | The slug of the mod found in the Mod DB URL. |
| `mods.version` | String | The specific version string to pin. |

---

## 4. Technical Constraints & Error Handling

* **Network**: Uses `http.Client` with a custom `User-Agent` to bypass CDN filtering and satisfy API requirements.
* **Concurrency**: Implements `sync.WaitGroup` and a buffered channel semaphore (default: 4) to limit thread saturation during IO-bound tasks.
* **Color Space**: Utilizes ANSI escape sequences. Standard "Blue" (34) is substituted with "Cyan" (36) or "Bright Blue" (94) for high visibility across various terminal themes.
* **Exit Codes**:
    * `0`: Success.
    * `1`: Fatal initialization or path permission error.
    * `non-zero`: API failure, network timeout, or nil-pointer prevention during file stat.

---

## 5. Development Roadmap

### Phase 1: Core Stability (Completed)
- [x] Concurrent worker pool for downloads.
- [x] Global CAS caching and symlink-based environment switching.
- [x] Prefix-based semantic version matching.
- [x] Graceful error handling for missing directories and nil pointers.

### Phase 2: Manifest & Local Registry (Active)
- [ ] **CLI Manifest Creator**: Commands `vsmod new [name]` and `vsmod add [slug]` to programmatically generate and edit TOML files in the local registry.
- [ ] **Conservative Cleanup**: Logic to prune redundant files from a pack directory (e.g., after a mod is removed from TOML) without destroying the entire directory state.
- [ ] **Integrity Verification**: SHA-256 checksum validation for cached assets.

### Phase 3: Advanced Automation
- [ ] **Mod DB Search**: Integrated CLI search to find mod slugs and versions without a browser.
- [ ] **Dependency Resolution**: Recursive logic to identify and fetch required library mods (e.g., `CommonLib`).
- [ ] **Multi-Instance Support**: Allow arbitrary mapping of game data paths for compatibility with third-party launchers.
