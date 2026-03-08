package nzb

import (
	"strings"
	"testing"
)

const validNZB = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.1//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.1.dtd">
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <head>
    <meta type="title">Test Download</meta>
    <meta type="password">secret123</meta>
  </head>
  <file poster="user@example.com" date="1609459200" subject='My.File.S01E01.720p "My.File.S01E01.720p.mkv" yEnc (1/10)'>
    <groups>
      <group>alt.binaries.test</group>
      <group>alt.binaries.misc</group>
    </groups>
    <segments>
      <segment bytes="500000" number="3">segment3@example.com</segment>
      <segment bytes="500000" number="1">segment1@example.com</segment>
      <segment bytes="250000" number="2">segment2@example.com</segment>
    </segments>
  </file>
</nzb>`

func TestParse_ValidNZB(t *testing.T) {
	nzb, err := Parse(strings.NewReader(validNZB))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(nzb.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(nzb.Files))
	}

	f := nzb.Files[0]
	if f.Poster != "user@example.com" {
		t.Errorf("Poster = %q", f.Poster)
	}
	if f.Date != 1609459200 {
		t.Errorf("Date = %d", f.Date)
	}
	if len(f.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(f.Groups))
	}
	if len(f.Segments) != 3 {
		t.Errorf("expected 3 segments, got %d", len(f.Segments))
	}
}

func TestParse_SegmentsSortedByNumber(t *testing.T) {
	nzb, err := Parse(strings.NewReader(validNZB))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	segs := nzb.Files[0].Segments
	for i := 1; i < len(segs); i++ {
		if segs[i].Number <= segs[i-1].Number {
			t.Errorf("segments not sorted: segment[%d].Number=%d <= segment[%d].Number=%d",
				i, segs[i].Number, i-1, segs[i-1].Number)
		}
	}
}

func TestParse_SegmentFields(t *testing.T) {
	nzb, err := Parse(strings.NewReader(validNZB))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	seg := nzb.Files[0].Segments[0] // Should be number=1 after sorting
	if seg.Number != 1 {
		t.Errorf("first segment Number = %d, want 1", seg.Number)
	}
	if seg.MessageID != "segment1@example.com" {
		t.Errorf("first segment MessageID = %q", seg.MessageID)
	}
	if seg.Bytes != 500000 {
		t.Errorf("first segment Bytes = %d", seg.Bytes)
	}
}

func TestParse_Metadata(t *testing.T) {
	nzb, err := Parse(strings.NewReader(validNZB))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if nzb.Meta["title"] != "Test Download" {
		t.Errorf("Meta[title] = %q", nzb.Meta["title"])
	}
	if nzb.Meta["password"] != "secret123" {
		t.Errorf("Meta[password] = %q", nzb.Meta["password"])
	}
}

func TestParse_MultipleFiles(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <head></head>
  <file poster="u@e.com" date="1000" subject="file1">
    <groups><group>alt.test</group></groups>
    <segments><segment bytes="100" number="1">a@b</segment></segments>
  </file>
  <file poster="u@e.com" date="2000" subject="file2">
    <groups><group>alt.test</group></groups>
    <segments><segment bytes="200" number="1">c@d</segment></segments>
  </file>
</nzb>`

	nzb, err := Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(nzb.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(nzb.Files))
	}
	if nzb.Files[0].Subject != "file1" {
		t.Errorf("Files[0].Subject = %q", nzb.Files[0].Subject)
	}
	if nzb.Files[1].Subject != "file2" {
		t.Errorf("Files[1].Subject = %q", nzb.Files[1].Subject)
	}
}

func TestParse_EmptyNZB(t *testing.T) {
	xml := `<?xml version="1.0"?><nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"><head></head></nzb>`
	_, err := Parse(strings.NewReader(xml))
	if err == nil {
		t.Error("expected error for NZB with no files")
	}
}

func TestParse_InvalidXML(t *testing.T) {
	_, err := Parse(strings.NewReader("this is not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestTotalBytes(t *testing.T) {
	nzb := &NZB{
		Files: []File{
			{Segments: []Segment{{Bytes: 100}, {Bytes: 200}}},
			{Segments: []Segment{{Bytes: 300}}},
		},
	}
	if got := nzb.TotalBytes(); got != 600 {
		t.Errorf("TotalBytes = %d, want 600", got)
	}
}

func TestTotalBytes_Empty(t *testing.T) {
	nzb := &NZB{}
	if got := nzb.TotalBytes(); got != 0 {
		t.Errorf("TotalBytes = %d, want 0", got)
	}
}

func TestTotalSegments(t *testing.T) {
	nzb := &NZB{
		Files: []File{
			{Segments: []Segment{{}, {}, {}}},
			{Segments: []Segment{{}, {}}},
		},
	}
	if got := nzb.TotalSegments(); got != 5 {
		t.Errorf("TotalSegments = %d, want 5", got)
	}
}

func TestFile_Filename_Quoted(t *testing.T) {
	f := File{Subject: `My.File.S01E01 "My.File.S01E01.mkv" yEnc (1/10)`}
	if got := f.Filename(); got != "My.File.S01E01.mkv" {
		t.Errorf("Filename = %q, want My.File.S01E01.mkv", got)
	}
}

func TestFile_Filename_Fallback(t *testing.T) {
	f := File{Subject: "My.File.S01E01 (01/15)"}
	if got := f.Filename(); got != "My.File.S01E01" {
		t.Errorf("Filename = %q, want My.File.S01E01", got)
	}
}

func TestFile_Filename_NoDelimiters(t *testing.T) {
	f := File{Subject: "simple-filename.rar"}
	if got := f.Filename(); got != "simple-filename.rar" {
		t.Errorf("Filename = %q, want simple-filename.rar", got)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/file.nzb")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
