package rss

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/phekno/gobin/internal/config"
	"github.com/phekno/gobin/internal/queue"
)

func TestParseRSS(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Show.S01E01.1080p</title>
      <link>http://example.com/nzb/1</link>
      <guid>guid-1</guid>
    </item>
    <item>
      <title>Movie.2024.UHD</title>
      <link>http://example.com/nzb/2</link>
      <guid>guid-2</guid>
      <enclosure url="http://example.com/nzb/2.nzb" type="application/x-nzb"/>
    </item>
  </channel>
</rss>`)

	items, err := parseRSS(data)
	if err != nil {
		t.Fatalf("parseRSS: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "Show.S01E01.1080p" {
		t.Errorf("item 0 title = %q", items[0].Title)
	}
	if items[1].Enclosure.URL != "http://example.com/nzb/2.nzb" {
		t.Errorf("item 1 enclosure URL = %q", items[1].Enclosure.URL)
	}
}

func TestMatchesFilters_NoFilters(t *testing.T) {
	if !matchesFilters("anything", nil) {
		t.Error("no filters should match everything")
	}
}

func TestMatchesFilters_Include(t *testing.T) {
	filters := []config.RSSFilter{{Include: "1080p"}}
	if !matchesFilters("Show.S01E01.1080p.BluRay", filters) {
		t.Error("should match include filter")
	}
	if matchesFilters("Show.S01E01.720p.BluRay", filters) {
		t.Error("should not match when include doesn't match")
	}
}

func TestMatchesFilters_Exclude(t *testing.T) {
	filters := []config.RSSFilter{{Exclude: "CAM"}}
	if !matchesFilters("Show.S01E01.1080p.BluRay", filters) {
		t.Error("should match when exclude doesn't match")
	}
	if matchesFilters("Show.S01E01.CAM", filters) {
		t.Error("should not match when exclude matches")
	}
}

func TestMatchesFilters_IncludeAndExclude(t *testing.T) {
	filters := []config.RSSFilter{
		{Include: "1080p"},
		{Exclude: "CAM"},
	}
	if !matchesFilters("Show.S01E01.1080p.BluRay", filters) {
		t.Error("should match: has 1080p, no CAM")
	}
	if matchesFilters("Show.S01E01.1080p.CAM", filters) {
		t.Error("should not match: has 1080p but also CAM")
	}
}

func TestMatchesFilters_CaseInsensitive(t *testing.T) {
	filters := []config.RSSFilter{{Include: "bluray"}}
	if !matchesFilters("Show.S01E01.BluRay", filters) {
		t.Error("should match case-insensitively")
	}
}

var testCounter int

func testID() string {
	testCounter++
	return fmt.Sprintf("rss-test-%d", testCounter)
}

func TestPollFeed(t *testing.T) {
	rssBody := `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item><title>Show.S01E01.1080p</title><link>http://example.com/1.nzb</link><guid>g1</guid></item>
  <item><title>Show.S01E02.720p</title><link>http://example.com/2.nzb</link><guid>g2</guid></item>
  <item><title>Movie.CAM.Bad</title><link>http://example.com/3.nzb</link><guid>g3</guid></item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	q := queue.NewManager(10)
	cfgMgr := config.NewManager(filepath.Join(t.TempDir(), "cfg.yaml"), &config.Config{
		RSS: config.RSS{
			Enabled:         true,
			IntervalMinutes: 15,
			Feeds: []config.RSSFeed{{
				Name:     "test",
				URL:      srv.URL,
				Category: "tv",
				Filters:  []config.RSSFilter{{Include: "1080p"}},
			}},
		},
	})

	p := New(cfgMgr, q, testID)
	err := p.pollFeed(context.Background(), cfgMgr.Get().RSS.Feeds[0])
	if err != nil {
		t.Fatalf("pollFeed: %v", err)
	}

	jobs := q.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job (only 1080p match), got %d", len(jobs))
	}
	if jobs[0].Name != "Show.S01E01.1080p" {
		t.Errorf("name = %q", jobs[0].Name)
	}
	if jobs[0].Category != "tv" {
		t.Errorf("category = %q", jobs[0].Category)
	}
}

func TestPollFeed_DeduplicatesOnSecondPoll(t *testing.T) {
	rssBody := `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item><title>Show.S01E01</title><link>http://example.com/1.nzb</link><guid>dedup-1</guid></item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	q := queue.NewManager(10)
	cfgMgr := config.NewManager(filepath.Join(t.TempDir(), "cfg.yaml"), &config.Config{
		RSS: config.RSS{Enabled: true, Feeds: []config.RSSFeed{{Name: "test", URL: srv.URL}}},
	})

	p := New(cfgMgr, q, testID)
	feed := cfgMgr.Get().RSS.Feeds[0]

	_ = p.pollFeed(context.Background(), feed)
	_ = p.pollFeed(context.Background(), feed) // second poll

	if len(q.List()) != 1 {
		t.Errorf("expected 1 job (dedup), got %d", len(q.List()))
	}
}
