package decoder

import (
	"fmt"
	"hash/crc32"
	"testing"
)

// encodeYEnc is a test helper that yEnc-encodes a byte slice.
func encodeYEnc(data []byte) []byte {
	var encoded []byte
	for _, b := range data {
		out := (b + 42) & 0xFF
		// Escape critical bytes: NUL, LF, CR, '=', TAB, SPACE
		switch out {
		case 0x00, 0x0A, 0x0D, 0x3D, 0x09, 0x20:
			encoded = append(encoded, '=', (out+64)&0xFF)
		default:
			encoded = append(encoded, out)
		}
	}
	return encoded
}

// buildYEncPayload constructs a complete yEnc article from plaintext data.
func buildYEncPayload(t *testing.T, name string, data []byte, includeCRC bool) []byte {
	t.Helper()
	encoded := encodeYEnc(data)
	crcVal := crc32.ChecksumIEEE(data)

	payload := fmt.Sprintf("=ybegin line=128 size=%d name=%s\r\n", len(data), name)
	payload += string(encoded) + "\r\n"
	if includeCRC {
		payload += fmt.Sprintf("=yend size=%d crc32=%08x\r\n", len(data), crcVal)
	} else {
		payload += fmt.Sprintf("=yend size=%d\r\n", len(data))
	}
	return []byte(payload)
}

func TestDecodeYEnc_SinglePart(t *testing.T) {
	plaintext := []byte("Hello, World!")
	raw := buildYEncPayload(t, "test.txt", plaintext, true)

	result, err := DecodeYEnc(raw)
	if err != nil {
		t.Fatalf("DecodeYEnc failed: %v", err)
	}
	if result.Filename != "test.txt" {
		t.Errorf("Filename = %q, want test.txt", result.Filename)
	}
	if string(result.Data) != string(plaintext) {
		t.Errorf("Data = %q, want %q", result.Data, plaintext)
	}
}

func TestDecodeYEnc_MultiPart(t *testing.T) {
	plaintext := []byte("part data")
	encoded := encodeYEnc(plaintext)
	crcVal := crc32.ChecksumIEEE(plaintext)

	raw := fmt.Sprintf("=ybegin part=2 total=5 line=128 size=1000 name=multi.bin\r\n"+
		"=ypart begin=5001 end=10000\r\n"+
		"%s\r\n"+
		"=yend size=%d part=2 pcrc32=%08x\r\n",
		string(encoded), len(plaintext), crcVal)

	result, err := DecodeYEnc([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeYEnc failed: %v", err)
	}
	if result.Part != 2 {
		t.Errorf("Part = %d, want 2", result.Part)
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	if result.Begin != 5001 {
		t.Errorf("Begin = %d, want 5001", result.Begin)
	}
	if result.End != 10000 {
		t.Errorf("End = %d, want 10000", result.End)
	}
	if string(result.Data) != string(plaintext) {
		t.Errorf("Data = %q, want %q", result.Data, plaintext)
	}
}

func TestDecodeYEnc_EscapeSequences(t *testing.T) {
	// Bytes that, after adding 42, produce critical chars needing escaping.
	// Critical encoded bytes: NUL(0x00), TAB(0x09), LF(0x0A), CR(0x0D), '='(0x3D), SPACE(0x20)
	// Original bytes that produce these: (critical - 42) & 0xFF
	plaintext := []byte{
		(0x00 - 42) & 0xFF, // → encoded 0x00 (NUL)
		(0x09 - 42) & 0xFF, // → encoded 0x09 (TAB)
		(0x0A - 42) & 0xFF, // → encoded 0x0A (LF)
		(0x0D - 42) & 0xFF, // → encoded 0x0D (CR)
		(0x3D - 42) & 0xFF, // → encoded 0x3D (=)
		(0x20 - 42) & 0xFF, // → encoded 0x20 (SPACE)
	}

	raw := buildYEncPayload(t, "escape.bin", plaintext, true)

	result, err := DecodeYEnc(raw)
	if err != nil {
		t.Fatalf("DecodeYEnc failed: %v", err)
	}
	if len(result.Data) != len(plaintext) {
		t.Fatalf("Data length = %d, want %d", len(result.Data), len(plaintext))
	}
	for i, b := range result.Data {
		if b != plaintext[i] {
			t.Errorf("Data[%d] = 0x%02x, want 0x%02x", i, b, plaintext[i])
		}
	}
}

func TestDecodeYEnc_CRCMismatch(t *testing.T) {
	plaintext := []byte("check me")
	encoded := encodeYEnc(plaintext)

	raw := fmt.Sprintf("=ybegin line=128 size=%d name=bad.bin\r\n"+
		"%s\r\n"+
		"=yend size=%d crc32=DEADBEEF\r\n",
		len(plaintext), string(encoded), len(plaintext))

	result, err := DecodeYEnc([]byte(raw))
	if err == nil {
		t.Fatal("expected CRC mismatch error")
	}
	if result == nil {
		t.Fatal("expected result to still be returned on CRC error")
	}
	if string(result.Data) != string(plaintext) {
		t.Errorf("Data = %q, want %q", result.Data, plaintext)
	}
}

func TestDecodeYEnc_NoCRC(t *testing.T) {
	plaintext := []byte("no checksum")
	raw := buildYEncPayload(t, "nocrc.bin", plaintext, false)

	result, err := DecodeYEnc(raw)
	if err != nil {
		t.Fatalf("DecodeYEnc failed: %v", err)
	}
	if string(result.Data) != string(plaintext) {
		t.Errorf("Data = %q, want %q", result.Data, plaintext)
	}
}

func TestDecodeYEnc_TooFewLines(t *testing.T) {
	_, err := DecodeYEnc([]byte("only one line"))
	if err == nil {
		t.Error("expected error for too few lines")
	}
}

func TestDecodeYEnc_EmptyBodyLines(t *testing.T) {
	plaintext := []byte("AB")
	encoded := encodeYEnc(plaintext)
	crcVal := crc32.ChecksumIEEE(plaintext)

	// Insert blank lines in the body
	raw := fmt.Sprintf("=ybegin line=128 size=%d name=gaps.bin\r\n"+
		"\r\n"+
		"%s\r\n"+
		"\r\n"+
		"=yend size=%d crc32=%08x\r\n",
		len(plaintext), string(encoded), len(plaintext), crcVal)

	result, err := DecodeYEnc([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeYEnc failed: %v", err)
	}
	if string(result.Data) != string(plaintext) {
		t.Errorf("Data = %q, want %q", result.Data, plaintext)
	}
}

func TestExtractField(t *testing.T) {
	tests := []struct {
		line, key, want string
	}{
		{"=ybegin line=128 size=1000 name=my file.txt", "name=", "my file.txt"},
		{"=ybegin line=128 size=1000 name=my file.txt", "size=", "1000"},
		{"=ybegin line=128 size=1000 name=my file.txt", "line=", "128"},
		{"=ybegin size=500", "name=", ""},
		{"=yend size=100 crc32=ABCD1234", "crc32=", "ABCD1234"},
	}
	for _, tt := range tests {
		got := extractField(tt.line, tt.key)
		if got != tt.want {
			t.Errorf("extractField(%q, %q) = %q, want %q", tt.line, tt.key, got, tt.want)
		}
	}
}

func TestParseYEndFooter_PrefersPCRC(t *testing.T) {
	crc, ok := parseYEndFooter("=yend size=100 pcrc32=AABB1122 crc32=00000000")
	if !ok {
		t.Fatal("expected CRC to be found")
	}
	if crc != 0xAABB1122 {
		t.Errorf("CRC = %08x, want AABB1122", crc)
	}
}

func TestParseYEndFooter_FallbackToCRC(t *testing.T) {
	crc, ok := parseYEndFooter("=yend size=100 crc32=DEADBEEF")
	if !ok {
		t.Fatal("expected CRC to be found")
	}
	if crc != 0xDEADBEEF {
		t.Errorf("CRC = %08x, want DEADBEEF", crc)
	}
}

func TestParseYEndFooter_NoCRC(t *testing.T) {
	_, ok := parseYEndFooter("=yend size=100")
	if ok {
		t.Error("expected no CRC found")
	}
}
