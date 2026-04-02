package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

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

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed modpacks",
	Run: func(cmd *cobra.Command, args []string) {
		packsDir := filepath.Join(STORAGE_PATH, "packs")

		entries, err := os.ReadDir(packsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("%sNo modpacks found. Install a modpack using the %svsmod install%s command.%s\n", CYellow, CBlue, CYellow, CReset)
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
	},
}
