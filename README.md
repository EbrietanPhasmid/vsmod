# vsmod 

`vsmod` is a CLI-based mod management utility for Vintage Story, developed in Go. It provides deterministic modpack environments through a global content-addressable storage (CAS) model and filesystem abstraction via symbolic links.

## 1. Architectural Overview

### 1.1 Filesystem Hierarchy
The tool manages state within a dedicated storage directory (default: `~/.vsmod`).

* **Global Cache (`~/.vsmod/cache/`)**: Stores unique mod binaries. Naming convention: `{mod_slug}_{version}_{original_filename}`.
* **Pack Storage (`~/.vsmod/packs/`)**: Contains immutable snapshots of specific modpacks. Each subdirectory is named `{pack_name}_{game_version}`.
* **Game Integration**: The tool modifies the `VintagestoryData/Mods` directory. If a physical directory exists, it is moved to `Mods.backup`. A symbolic link is then created pointing from `VintagestoryData/Mods` to the active directory in `~/.vsmod/packs/`.

### 1.2 Execution Pipeline
1.  **Manifest Parsing**: Decodes TOML and validates schema requirements.
2.  **API Resolution**: Queries `https://mods.vintagestory.at/api/mod/{slug}` to retrieve release metadata.
3.  **Compatibility Analysis**: Performs prefix-based version matching (e.g., Game Version `1.21` satisfies `1.21.6`).
4.  **Concurrent Retrieval**: Utilizes a worker pool (semaphore-gated) to download assets.
5.  **Environment Mutation**: Performs an atomic swap of the `Mods` symlink to point to the resolved pack directory.

---

## 2. Command Reference

### `install [path/to/manifest.toml]`
The primary command for environment synchronization.
* **Validation**: Ensures the manifest contains a name and at least one mod.
* **Sync Logic**: Checks the global cache for existing binaries via `os.Stat`. Downloads only missing assets.
* **IO Operations**: Copies assets from the global cache to the pack-specific directory to ensure pack isolation.
* **Symlink Safety**: If `Mods` is a directory, it is backed up. If it is an existing symlink, it is unlinked and recreated to point to the new target.

### `version`
Outputs the current build string. Used for debugging and ensuring CLI parity with the Vintage Story API expectations.

### `clear cache`
Recursively removes all files within `~/.vsmod/cache/`. This is a destructive operation used to reclaim disk space or resolve corrupted downloads.

---

## 3. Manifest Schema (TOML)

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | String | Unique identifier for the modpack. |
| `game_version` | String | Target Vintage Story version (used for compatibility logic). |
| `[[mods]]` | Array | A collection of mod objects. |
| `mods.name` | String | The slug of the mod as found in the Mod DB URL. |
| `mods.version` | String | The specific version string to pin. |

---

## 4. Technical Constraints & Error Handling

* **Network**: Uses `http.Client` with a custom `User-Agent` to bypass CDN filtering.
* **Concurrency**: Implements `sync.WaitGroup` and a buffered channel semaphore to limit hardware thread saturation during IO-bound tasks.
* **Color Space**: Utilizes ANSI escape sequences (`\033[...]`). Compatible with xterm-256color; standard "Blue" (34) is substituted with "Cyan" (36) or "Bright Blue" (94) for visibility across dark-mode terminal themes.
* **Exit Codes**:
    * `0`: Success.
    * `1`: Fatal initialization/Path error.
    * `non-zero`: API failure or IO permission error.

---

## 5. Development Roadmap

### Phase 1: Core Stability (Current)
- [x] Concurrent download engine.
- [x] Global caching and symlink-based pack switching.
- [x] Semantic version prefix matching.

### Phase 2: Optimization & Local State
- [ ] **Conservative Cleanup**: Implement logic to remove files from a pack directory that are no longer present in the TOML manifest, without re-downloading the entire pack.
- [ ] **Mod Refresh**: Command to check for updates based on the `game_version` without manual TOML editing.
- [ ] **Hashing**: Implement SHA-256 checksum validation for cached binaries to ensure integrity.

### Phase 3: Advanced Features
- [ ] **Dependency Resolution**: Automatically pull in required library mods (e.g., `CommonLib`) if omitted from the TOML.
- [ ] **Export/Import**: Generate a shareable `.vsmod` archive containing the TOML and a metadata lockfile.
- [ ] **Multi-Instance Support**: Allow mapping symlinks to non-standard `VintagestoryData` paths for multi-instance launchers.
