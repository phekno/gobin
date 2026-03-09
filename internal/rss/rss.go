// Package rss polls RSS feeds from Usenet indexers and auto-adds matching NZBs.
package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/queue"
)

// Poller periodically fetches RSS feeds and adds matching NZBs to the queue.
type Poller struct {
	cfgMgr *config.Manager
	queue  *queue.Manager
	client *http.Client
	idGen  func() string
	seen   map[string]bool // Track already-processed items by GUID
}

// New creates an RSS poller.
func New(cfgMgr *config.Manager, q *queue.Manager, idGen func() string) *Poller {
	return &Poller{
		cfgMgr: cfgMgr,
		queue:  q,
		client: &http.Client{Timeout: 30 * time.Second},
		idGen:  idGen,
		seen:   make(map[string]bool),
	}
}

// Run starts polling. Blocks until context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	cfg := p.cfgMgr.Get()
	if !cfg.RSS.Enabled {
		slog.Info("RSS polling disabled")
		return
	}

	interval := time.Duration(cfg.RSS.IntervalMinutes) * time.Minute
	if interval < time.Minute {
		interval = 15 * time.Minute
	}

	slog.Info("RSS polling started", "interval", interval, "feeds", len(cfg.RSS.Feeds))

	// Initial poll
	p.pollAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	cfg := p.cfgMgr.Get()
	for _, feed := range cfg.RSS.Feeds {
		if ctx.Err() != nil {
			return
		}
		if err := p.pollFeed(ctx, feed); err != nil {
			slog.Error("RSS feed poll failed", "feed", feed.Name, "error", err)
		}
	}
}

func (p *Poller) pollFeed(ctx context.Context, feed config.RSSFeed) error {
	slog.Debug("polling RSS feed", "feed", feed.Name, "url", feed.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "GoBin/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feed returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading feed body: %w", err)
	}

	items, err := parseRSS(body)
	if err != nil {
		return fmt.Errorf("parsing feed: %w", err)
	}

	added := 0
	for _, item := range items {
		// Skip already-seen items
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if p.seen[guid] {
			continue
		}
		p.seen[guid] = true

		// Apply filters
		if !matchesFilters(item.Title, feed.Filters) {
			continue
		}

		// Find the NZB URL (link or enclosure)
		nzbURL := item.Link
		if item.Enclosure.URL != "" {
			nzbURL = item.Enclosure.URL
		}
		if nzbURL == "" {
			continue
		}

		name := strings.TrimSuffix(item.Title, ".nzb")
		job := &queue.Job{
			ID:       p.idGen(),
			Name:     name,
			Category: feed.Category,
			NZBPath:  nzbURL, // Engine will need to handle URL-based NZB paths
		}

		if err := p.queue.Add(job); err != nil {
			slog.Warn("RSS: failed to add item", "title", item.Title, "error", err)
			continue
		}

		slog.Info("RSS: added NZB", "feed", feed.Name, "title", item.Title, "category", feed.Category)
		added++
	}

	if added > 0 {
		slog.Info("RSS poll complete", "feed", feed.Name, "added", added)
	}

	return nil
}

// matchesFilters checks if a title passes include/exclude regex filters.
func matchesFilters(title string, filters []config.RSSFilter) bool {
	for _, f := range filters {
		if f.Include != "" {
			matched, err := regexp.MatchString("(?i)"+f.Include, title)
			if err != nil || !matched {
				return false
			}
		}
		if f.Exclude != "" {
			matched, err := regexp.MatchString("(?i)"+f.Exclude, title)
			if err == nil && matched {
				return false
			}
		}
	}
	return true
}

// --- RSS XML parsing ---

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string       `xml:"title"`
	Link      string       `xml:"link"`
	GUID      string       `xml:"guid"`
	Enclosure rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

func parseRSS(data []byte) ([]rssItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	return feed.Channel.Items, nil
}
