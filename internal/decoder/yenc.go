package decoder

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
)

// YEncResult holds the decoded output of a yEnc-encoded article.
type YEncResult struct {
	Filename string
	Part     int
	Total    int
	Begin    int64 // Byte offset start (for multipart)
	End      int64 // Byte offset end (for multipart)
	Data     []byte
	CRC32    uint32
}

// DecodeYEnc decodes a yEnc-encoded byte slice (single or multi-part).
//
// yEnc encoding overview:
//   - Header line: =ybegin part=N line=128 size=XXXXX name=filename.ext
//   - Part header: =ypart begin=N end=N
//   - Encoded body: each byte = (original + 42) mod 256, with escape sequences
//   - Footer: =yend size=N part=N pcrc32=XXXXXXXX crc32=XXXXXXXX
//
// Critical bytes are escaped with '=' followed by (byte + 64) mod 256.
// Escaped characters: NUL (0x00), LF (0x0A), CR (0x0D), '=' (0x3D), TAB (0x09), SPACE (0x20)
func DecodeYEnc(raw []byte) (*YEncResult, error) {
	result := &YEncResult{}

	lines := bytes.Split(raw, []byte("\n"))
	if len(lines) < 3 {
		return nil, fmt.Errorf("yenc: too few lines (%d)", len(lines))
	}

	// Find and parse =ybegin header
	bodyStart := 0
	for i, line := range lines {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("=ybegin ")) {
			parseYBeginHeader(string(line), result)
			bodyStart = i + 1
			break
		}
	}

	// Check for =ypart header (multipart)
	if bodyStart < len(lines) {
		line := bytes.TrimRight(lines[bodyStart], "\r")
		if bytes.HasPrefix(line, []byte("=ypart ")) {
			parseYPartHeader(string(line), result)
			bodyStart++
		}
	}

	// Find =yend footer and extract body lines
	bodyEnd := len(lines)
	var expectedCRC uint32
	var hasCRC bool
	for i := len(lines) - 1; i >= bodyStart; i-- {
		line := bytes.TrimRight(lines[i], "\r")
		if bytes.HasPrefix(line, []byte("=yend ")) {
			bodyEnd = i
			expectedCRC, hasCRC = parseYEndFooter(string(line))
			break
		}
	}

	// Decode the body
	// Pre-allocate output buffer (estimate ~same size as input)
	decoded := make([]byte, 0, bodyEnd-bodyStart)

	for i := bodyStart; i < bodyEnd; i++ {
		line := bytes.TrimRight(lines[i], "\r\n")
		if len(line) == 0 {
			continue
		}

		decoded = decodeLine(line, decoded)
	}

	result.Data = decoded

	// Verify CRC32 if present
	if hasCRC {
		actual := crc32.ChecksumIEEE(decoded)
		result.CRC32 = actual
		if actual != expectedCRC {
			return result, fmt.Errorf("yenc: CRC32 mismatch: expected %08x, got %08x",
				expectedCRC, actual)
		}
	}

	return result, nil
}

// decodeLine decodes a single yEnc-encoded line, appending to dst.
// This is the hot path — kept tight for performance.
func decodeLine(line []byte, dst []byte) []byte {
	i := 0
	n := len(line)

	for i < n {
		b := line[i]

		if b == '=' {
			// Escape sequence: next byte is (char - 64) then subtract 42
			i++
			if i >= n {
				break
			}
			b = (line[i] - 64 - 42) & 0xFF
		} else {
			// Normal byte: subtract 42
			b = (b - 42) & 0xFF
		}

		dst = append(dst, b)
		i++
	}

	return dst
}

// parseYBeginHeader extracts fields from: =ybegin part=1 line=128 size=123456 name=file.ext
func parseYBeginHeader(header string, result *YEncResult) {
	if v := extractField(header, "name="); v != "" {
		result.Filename = v
	}
	if v := extractField(header, "part="); v != "" {
		result.Part, _ = strconv.Atoi(v)
	}
	if v := extractField(header, "total="); v != "" {
		result.Total, _ = strconv.Atoi(v)
	}
}

// parseYPartHeader extracts fields from: =ypart begin=1 end=768000
func parseYPartHeader(header string, result *YEncResult) {
	if v := extractField(header, "begin="); v != "" {
		result.Begin, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := extractField(header, "end="); v != "" {
		result.End, _ = strconv.ParseInt(v, 10, 64)
	}
}

// parseYEndFooter extracts CRC from: =yend size=768000 part=1 pcrc32=ABCD1234 crc32=ABCD1234
func parseYEndFooter(footer string) (uint32, bool) {
	// Prefer pcrc32 (part CRC) for multipart, fall back to crc32
	for _, key := range []string{"pcrc32=", "crc32="} {
		if v := extractField(footer, key); v != "" {
			val, err := strconv.ParseUint(v, 16, 32)
			if err == nil {
				return uint32(val), true
			}
		}
	}
	return 0, false
}

// extractField pulls a value from a header line like "key=value".
// For "name=" it takes everything after the key to end of line (filenames can have spaces).
// For other keys it takes until the next space.
func extractField(line, key string) string {
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	val := line[idx+len(key):]

	// "name=" is special: value continues to end of line
	if key == "name=" {
		return strings.TrimSpace(val)
	}

	// Other fields end at the next space
	if sp := strings.IndexByte(val, ' '); sp >= 0 {
		val = val[:sp]
	}
	return strings.TrimSpace(val)
}
