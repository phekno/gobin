package nzb

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// NZB represents a parsed NZB file containing one or more files to download.
type NZB struct {
	Meta  map[string]string
	Files []File
}

// File represents a single file within an NZB, made up of segments.
type File struct {
	Poster  string
	Date    int64
	Subject string
	Groups  []string
	Segments []Segment
}

// Segment represents a single article/segment of a file on Usenet.
type Segment struct {
	Number    int
	Bytes     int64
	MessageID string // The Message-ID used to fetch via NNTP
}

// TotalBytes returns the total expected size of the NZB in bytes.
func (n *NZB) TotalBytes() int64 {
	var total int64
	for _, f := range n.Files {
		for _, s := range f.Segments {
			total += s.Bytes
		}
	}
	return total
}

// TotalSegments returns the total number of segments across all files.
func (n *NZB) TotalSegments() int {
	var total int
	for _, f := range n.Files {
		total += len(f.Segments)
	}
	return total
}

// Filename extracts the probable filename from the subject line.
// NZB subjects typically look like: 'My.File.mkv (01/15) "My.File.mkv" yEnc (1/15)'
func (f *File) Filename() string {
	// Try to extract quoted filename first
	start := strings.Index(f.Subject, `"`)
	if start >= 0 {
		end := strings.Index(f.Subject[start+1:], `"`)
		if end >= 0 {
			return f.Subject[start+1 : start+1+end]
		}
	}
	// Fallback: use subject up to first space or bracket
	subj := f.Subject
	for _, delim := range []string{" (", " [", " -"} {
		if idx := strings.Index(subj, delim); idx > 0 {
			subj = subj[:idx]
		}
	}
	return strings.TrimSpace(subj)
}

// --- XML structures for parsing ---

type xmlNZB struct {
	XMLName xml.Name  `xml:"nzb"`
	Head    xmlHead   `xml:"head"`
	Files   []xmlFile `xml:"file"`
}

type xmlHead struct {
	Meta []xmlMeta `xml:"meta"`
}

type xmlMeta struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type xmlFile struct {
	Poster  string      `xml:"poster,attr"`
	Date    string      `xml:"date,attr"`
	Subject string      `xml:"subject,attr"`
	Groups  xmlGroups   `xml:"groups"`
	Segments xmlSegments `xml:"segments"`
}

type xmlGroups struct {
	Group []string `xml:"group"`
}

type xmlSegments struct {
	Segment []xmlSegment `xml:"segment"`
}

type xmlSegment struct {
	Number string `xml:"number,attr"`
	Bytes  string `xml:"bytes,attr"`
	ID     string `xml:",chardata"`
}

// ParseFile parses an NZB file from disk.
func ParseFile(path string) (*NZB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening nzb: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// Parse reads and parses an NZB from any reader.
// Uses streaming XML decoding to handle large NZB files without
// loading the entire DOM into memory.
func Parse(r io.Reader) (*NZB, error) {
	var raw xmlNZB
	decoder := xml.NewDecoder(r)
	decoder.Strict = false // Be lenient with malformed NZBs
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding nzb xml: %w", err)
	}

	nzb := &NZB{
		Meta:  make(map[string]string),
		Files: make([]File, 0, len(raw.Files)),
	}

	// Parse metadata
	for _, m := range raw.Head.Meta {
		nzb.Meta[m.Type] = m.Value
	}

	// Parse files and segments
	for _, rf := range raw.Files {
		date, _ := strconv.ParseInt(rf.Date, 10, 64)

		file := File{
			Poster:   rf.Poster,
			Date:     date,
			Subject:  rf.Subject,
			Groups:   rf.Groups.Group,
			Segments: make([]Segment, 0, len(rf.Segments.Segment)),
		}

		for _, rs := range rf.Segments.Segment {
			num, _ := strconv.Atoi(rs.Number)
			bytes, _ := strconv.ParseInt(rs.Bytes, 10, 64)

			file.Segments = append(file.Segments, Segment{
				Number:    num,
				Bytes:     bytes,
				MessageID: strings.TrimSpace(rs.ID),
			})
		}

		// Sort segments by number for sequential assembly
		sort.Slice(file.Segments, func(i, j int) bool {
			return file.Segments[i].Number < file.Segments[j].Number
		})

		nzb.Files = append(nzb.Files, file)
	}

	if len(nzb.Files) == 0 {
		return nil, fmt.Errorf("nzb contains no files")
	}

	return nzb, nil
}
