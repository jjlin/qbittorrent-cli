package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/ludviglundgren/qbittorrent-cli/internal/config"

	"github.com/autobrr/go-qbittorrent"
	"github.com/deckarep/golang-set/v2"
	"github.com/spf13/cobra"
)

// RunTorrentRemove cmd to remove torrents
func RunTorrentRemove() *cobra.Command {
	var (
		dryRun          bool
		removeAll       bool
		deleteFiles     bool
		hashes          []string
		includeCategory []string
		includeTags     []string
		excludeTags     []string
		filter          string
	)

	var command = &cobra.Command{
		Use:   "remove",
		Short: "Removes specified torrent(s)",
		Long:  `Removes torrents indicated by hash, name or a prefix of either. Whitespace indicates next prefix unless argument is surrounded by quotes`,
	}

	command.Flags().BoolVar(&dryRun, "dry-run", false, "Display what would be done without actually doing it")
	command.Flags().BoolVar(&removeAll, "all", false, "Removes all torrents")
	command.Flags().BoolVar(&deleteFiles, "delete-files", false, "Also delete downloaded files from torrent(s)")
	command.Flags().StringVar(&filter, "filter", "", "Filter by state: all, downloading, seeding, completed, paused, active, inactive, resumed, \nstalled, stalled_uploading, stalled_downloading, errored")
	command.Flags().StringSliceVar(&hashes, "hashes", []string{}, "Add hashes as comma separated list")
	command.Flags().StringSliceVar(&includeCategory, "include-category", []string{}, "Remove torrents from these categories. Comma separated")
	command.Flags().StringSliceVar(&includeTags, "include-tags", []string{}, "Include torrents with provided tags")
	command.Flags().StringSliceVar(&excludeTags, "exclude-tags", []string{}, "Exclude torrents with provided tags")

	command.Run = func(cmd *cobra.Command, args []string) {
		config.InitConfig()

		qbtSettings := qbittorrent.Config{
			Host:      config.Qbit.Addr,
			Username:  config.Qbit.Login,
			Password:  config.Qbit.Password,
			BasicUser: config.Qbit.BasicUser,
			BasicPass: config.Qbit.BasicPass,
		}

		qb := qbittorrent.NewClient(qbtSettings)

		ctx := cmd.Context()

		if err := qb.LoginCtx(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: connection failed: %v\n", err)
			os.Exit(1)
		}

		options := qbittorrent.TorrentFilterOptions{}

		var filterHashes mapset.Set[string]
		if filter != "" {
			filterHashes = mapset.NewSet[string]()
			options.Filter = qbittorrent.TorrentFilter(filter)
			torrents, err := qb.GetTorrentsCtx(ctx, options)
			if err != nil {
				log.Fatalf("could not get torrents for filter: %s err: %q\n", filter, err)
			}
			for _, torrent := range torrents {
				filterHashes.Add(torrent.Hash)
			}
		}

		var catTagHashes mapset.Set[string]
		if len(includeCategory) > 0 {
			catTagHashes = mapset.NewSet[string]()
			for _, category := range includeCategory {
				options.Category = category

				torrents, err := qb.GetTorrentsCtx(ctx, options)
				if err != nil {
					log.Fatalf("could not get torrents for category: %s err: %q\n", category, err)
				}

				for _, torrent := range torrents {
					if len(includeTags) > 0 {
						if _, hasTag := validateTag(includeTags, torrent.Tags); !hasTag {
							continue
						}
					}

					if len(excludeTags) > 0 {
						if _, hasTag := validateTag(excludeTags, torrent.Tags); hasTag {
							continue
						}
					}

					catTagHashes.Add(torrent.Hash)
				}
			}
		}

		if removeAll {
			hashes = []string{"all"}
		} else {
			// Merge all matching hashes.
			var hashSet mapset.Set[string]

			if filterHashes != nil && catTagHashes != nil {
				hashSet = filterHashes.Intersect(catTagHashes)
			} else if filterHashes != nil {
				hashSet = filterHashes
			} else {
				hashSet = catTagHashes
			}

			specifiedHashes := mapset.NewSet[string](hashes...)
			hashes = specifiedHashes.Union(hashSet).ToSlice()
		}

		if len(hashes) == 0 && !removeAll {
			log.Println("No torrents found to remove")
			return
		}

		if dryRun {
			if removeAll {
				log.Println("dry-run: all torrents to be removed")
			} else {
				log.Printf("dry-run: (%d) torrents to be removed\n", len(hashes))
			}
		} else {
			if removeAll {
				log.Println("all torrents to be removed")
			} else {
				log.Printf("(%d) torrents to be removed\n", len(hashes))
			}

			err := batchRequests(hashes, func(start, end int) error {
				return qb.DeleteTorrentsCtx(ctx, hashes[start:end], deleteFiles)
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not delete torrents: %v\n", err)
				os.Exit(1)
				return
			}

			if removeAll {
				log.Println("successfully removed all torrents")
			} else {
				log.Printf("successfully removed (%d) torrents\n", len(hashes))
			}
		}
	}

	return command
}
