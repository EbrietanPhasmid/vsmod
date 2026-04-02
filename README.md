# vsmod 🛠️

A high-performance, concurrent mod manager for **Vintage Story**. 

`vsmod` allows you to define your modpacks in simple TOML files. It handles parallel downloads, global caching to save bandwidth, and uses symlinks to swap between modpacks instantly without moving gigabytes of data.

## ✨ Features

* **Concurrent Downloads:** Powered by Go routines and `mpb` for beautiful, multi-bar progress tracking.
* **Intelligent Caching:** Downloads mods once to a global cache. If multiple packs use the same mod version, it's instant.
* **Symlink Management:** Automatically handles your `VintagestoryData/Mods` folder. It backs up your existing mods and swaps in your pack via symlinks.
* **Version Guard:** Checks API metadata to warn you if a mod version is incompatible with your pack's game version, even suggesting the correct version to use.
* **Cross-Platform:** Native support for Windows (`%AppData%`), Linux (`~/.config`), and macOS.

## 🚀 Quick Start

### Installation
If you have Go installed:
```bash
go install [github.com/yourusername/vsmod@latest](https://github.com/yourusername/vsmod@latest)
