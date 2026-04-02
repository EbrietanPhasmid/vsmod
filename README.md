# vsmod 
`vsmod` is a high-performance CLI mod manager for Vintage Story, engineered in Go. It provides deterministic, near-instantaneous environment switching through a global content-addressable storage (CAS) model and zero-copy filesystem linking.

## 1. Technical Architecture

### 1.1 Storage Strategy
The tool decouples mod data from the game directory to maintain a clean system state within `~/.vsmod`.

* **Metadata Registry (`registry.json`)**: A local cache of Mod DB API responses. This enables **"Fast Path"** execution by bypassing network requests for known mod/version pairs.
* **Global Cache (`/cache/`)**: The Source of Truth for unique mod binaries.
* **Pack Storage (`/packs/`)**: Discrete directories representing the resolved state of a TOML manifest.
* **Zero-Copy Hardlinking**: Instead of standard file copying, `vsmod` makes use of hard links to point files from the global cache to the pack folder. This is an O(1) operation that consumes no additional disk space.

### 1.2 Execution Pipeline
1.  **Registry Lookup**: Scans the local registry for mod metadata to avoid API latency.
2.  **Lazy Compatibility Heuristic**: Compares cached game versions against the target. If a **Major/Minor** mismatch is detected, it triggers an isolated API call to fetch version suggestions.
3.  **Parallel Sync**: Uses a semaphore-gated worker pool to download missing assets into the cache.
4.  **Conservative Cleanup**: Prunes files from the pack directory that are no longer present in the TOML, ensuring the environment remains lean without full re-installs.
5.  **Atomic Symlinking**: Swaps the game's `Mods` directory for a symbolic link pointing to the desired pack.

---

## 2. Command Reference

### `vsmod install [file.toml]`
Synchronizes your local filesystem to match a modpack manifest.
* **How it works**: It audits your cache, downloads what’s missing, and links the results into a dedicated pack folder.
* **Performance**: Achieves high speeds (<1s) for cached packs by utilizing the local metadata registry.
* **First Run**: If your game has a physical `Mods` folder, `vsmod` safely moves it to `Mods.backup` before creating the initial link.

### `vsmod link [pack_name]` 
Instantly changes which modpack the game sees.
* **Usage**: `vsmod link MySurvivalServer`
* **Why**: It simply updates a symlink. There is no file moving or copying involved; the change is effective the moment you launch the game.

### `vsmod list`
Displays all installed packs and identifies the active environment.
* **UI**: The currently linked pack is highlighted in **Cyan** with a `+` marker.
* **Accuracy**: It queries the filesystem's `readlink` state to ensure the UI represents the actual Source of Truth.

### `vsmod clear [cache | all]`
Reclaims disk space by removing stored data.
* **cache**: Cleans out the binary downloads in `~/.vsmod/cache/`.
* **all**: Destroys the entire `~/.vsmod` directory, including the registry and all pack folders.

---

## 3. Manifest Schema (TOML)

```toml
name = "ExamplePack"
game_version = "1.21.1"

[[mods]]
name = "carryon"
version = "1.13.0"

[[mods]]
name = "prospecttogether"
version = "1.3.0"
```

---

## 4. Performance & Constraints

* **Concurrency**: Optimized for 4 concurrent workers to maximize I/O throughput without triggering Mod DB rate limits.
* **Network Optimization**: Uses persistent HTTP connections (Keep-Alive) to reduce TLS handshake overhead.
* **SemVer Logic**: Implements a "Patch-Agnostic" warning system. It only performs expensive network-driven version suggestions if a breaking version jump is detected.
* **Exit Codes**:
    * `0`: Success.
    * `1`: Fatal error (Permissions, API downtime).
    * `2`: Manifest validation or version mismatch error.

---

## 5. Development Roadmap

### Phase 1: Core Performance (Completed)
- [x] Concurrent worker pool & Global CAS.
- [x] Symlink-based environment hotswapping.
- [x] **Hardlinking** for zero-copy file distribution.
- [x] **Local Metadata Registry** for sub-second execution.

### Phase 2: CLI UX (Active)
- [x] **Conservative Cleanup**: Logic to prune removed mods without full re-downloads.
- [ ] **Interactive Add/Remove**: Update TOML manifests directly via the CLI.
- [ ] **Init Command**: Bootstrap a new manifest from the current game state.

### Phase 3: Intelligence (Backlog)
- [ ] **Dependency Resolution**: Recursive logic to identify and fetch required library mods (e.g., `CommonLib`).
- [ ] **Mod DB Search**: Integrated CLI search to find mod slugs and versions.
