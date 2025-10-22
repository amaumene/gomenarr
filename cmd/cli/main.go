package main

import (
	"context"
	"fmt"
	"os"

	"github.com/amaumene/gomenarr/internal/infra"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gomenarr-cli",
		Short: "Gomenarr CLI for manual operations",
	}

	var syncCmd = &cobra.Command{
		Use:   "sync",
		Short: "Sync media from Trakt",
		Run: func(cmd *cobra.Command, args []string) {
			app, err := infra.InitializeApplication()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to initialize application")
			}

			ctx := context.Background()

			fmt.Println("Syncing movies...")
			if err := app.MediaService.SyncMovies(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to sync movies")
			}

			fmt.Println("Syncing episodes...")
			if err := app.MediaService.SyncEpisodes(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to sync episodes")
			}

			fmt.Println("Sync complete!")
		},
	}

	var cleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup watched media",
		Run: func(cmd *cobra.Command, args []string) {
			app, err := infra.InitializeApplication()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to initialize application")
			}

			ctx := context.Background()

			fmt.Println("Cleaning up watched media...")
			if err := app.CleanupService.CleanupWatched(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to cleanup")
			}

			fmt.Println("Cleanup complete!")
		},
	}

	rootCmd.AddCommand(syncCmd, cleanupCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
