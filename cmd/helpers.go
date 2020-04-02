package cmd

import (
	"github.com/dustin/go-humanize"
	"github.com/l3uddz/tqm/client"
	"github.com/l3uddz/tqm/config"
	"github.com/l3uddz/tqm/torrentfilemap"
	"github.com/sirupsen/logrus"
	"time"
)

// remove torrents that meet ignore filters
func removeIgnoredTorrents(log *logrus.Entry, c client.Interface, torrents map[string]config.Torrent) error {
	// vars
	ignoredTorrents := 0

	// iterate torrents
	for h, t := range torrents {
		ignore, err := c.ShouldIgnore(&t)
		if err != nil {
			// error while determining whether to ignore torrent
			log.WithError(err).Errorf("Failed determining whether to ignore %q: %+v", t.Name, t)
			delete(torrents, h)
			continue
		} else if ignore {
			// torrent met ignore filter
			log.Tracef("Ignoring torrent %s: %s", h, t.Name)
			delete(torrents, h)
			ignoredTorrents++
			continue
		}
	}

	log.Infof("Ignored %d torrents, %d left", ignoredTorrents, len(torrents))
	return nil
}

// remove torrents that meet remove filters
func removeEligibleTorrents(log *logrus.Entry, c client.Interface, torrents map[string]config.Torrent,
	tfm *torrentfilemap.TorrentFileMap) error {
	// vars
	softRemoveTorrents := 0
	hardRemoveTorrents := 0
	errorRemoveTorrents := 0
	var removedTorrentBytes int64 = 0

	// iterate torrents
	for h, t := range torrents {
		// should we remove this torrent?
		remove, err := c.ShouldRemove(&t)
		if err != nil {
			log.WithError(err).Errorf("Failed determining whether to remove %q: %+v", t.Name, t)
			// dont do any further operations on this torrent, but keep in the torrent file map
			delete(torrents, h)
			continue
		} else if !remove {
			// torrent did not meet the remove filters
			log.Tracef("Not removing %s: %s", h, t.Name)
			continue
		}

		// torrent meets the remove filters, are the files unique and eligible for a hard deletion (remove data)
		if uniqueTorrent := tfm.IsUnique(t); uniqueTorrent {
			// hard remove (the file paths in this torrent are unique to this torrent only)
			log.Info("-----")
			log.Infof("Hard removing: %q - %s", t.Name, humanize.IBytes(uint64(t.DownloadedBytes)))
			log.Infof("Ratio: %.3f / Seed days: %.3f / Seeds: %d / Label: %s / Tracker: %s / Tracker Status: %q",
				t.Ratio, t.SeedingDays, t.Seeds, t.Label, t.TrackerName, t.TrackerStatus)

			if !flagDryRun {
				// do remove
				removed, err := c.RemoveTorrent(t.Hash, true)
				if err != nil {
					log.WithError(err).Fatalf("Failed removing torrent: %+v", t)
					// dont remove from torrents file map, but prevent further operations on this torrent
					delete(torrents, h)
					errorRemoveTorrents++
					continue
				} else if !removed {
					log.Error("Failed removing torrent...")
					// dont remove from torrents file map, but prevent further operations on this torrent
					delete(torrents, h)
					errorRemoveTorrents++
					continue
				} else {
					log.Info("Removed")
					time.Sleep(2 * time.Second)
				}
			} else {
				log.Warn("Dry-run enabled, skipping remove...")
			}

			removedTorrentBytes += t.DownloadedBytes
			hardRemoveTorrents++

			// remove the torrent from the torrent file map
			tfm.Remove(t)
			delete(torrents, h)

		} else {
			// soft remove (there are other torrents with identical file paths)
			log.Info("-----")
			log.Warnf("Soft removing: %q - %s", t.Name, humanize.IBytes(uint64(t.DownloadedBytes)))
			log.Warnf("Ratio: %.3f / Seed days: %.3f / Seeds: %d / Label: %s / Tracker: %s / Tracker Status: %q",
				t.Ratio, t.SeedingDays, t.Seeds, t.Label, t.TrackerName, t.TrackerStatus)

			if !flagDryRun {
				// do remove
				removed, err := c.RemoveTorrent(t.Hash, false)
				if err != nil {
					log.WithError(err).Fatalf("Failed removing torrent: %+v", t)
					// dont remove from torrents file map, but prevent further operations on this torrent
					delete(torrents, h)
					errorRemoveTorrents++
					continue
				} else if !removed {
					log.Error("Failed removing torrent...")
					// dont remove from torrents file map, but prevent further operations on this torrent
					delete(torrents, h)
					errorRemoveTorrents++
					continue
				} else {
					log.Warn("Removed")
					time.Sleep(5 * time.Second)
				}
			} else {
				log.Warn("Dry-run enabled, skipping remove...")
			}

			softRemoveTorrents++

			// remove the torrent from the torrent file map
			tfm.Remove(t)
			delete(torrents, h)
		}
	}

	// show result
	log.Info("-----")
	log.WithField("reclaimed_space", humanize.IBytes(uint64(removedTorrentBytes))).
		Infof("Removed torrents: %d hard, %d soft and %d failures",
			hardRemoveTorrents, softRemoveTorrents, errorRemoveTorrents)
	return nil
}