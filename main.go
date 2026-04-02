package main

// TODO
// - conservative cleanup
// - split into smaller files

import (
	"fmt"
	"os"
	"time"
)

func init() {
	if err := setupPaths(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error setting up paths: %v\n", err)
		os.Exit(1)
	}
}

func main() {
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
