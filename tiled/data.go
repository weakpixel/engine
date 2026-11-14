package tiled

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// decodeData decodes the tile data of a layer or chunk into n raw GIDs.
// encoding is "csv" (a JSON array, the default) or "base64"; compression is
// "" (none), "zlib" or "gzip". zstd is not supported.
func decodeData(raw json.RawMessage, encoding, compression string, n int) ([]uint32, error) {
	switch encoding {
	case "", "csv":
		if compression != "" {
			return nil, fmt.Errorf("compression %q needs base64 encoding", compression)
		}
		var gids []uint32
		if err := json.Unmarshal(raw, &gids); err != nil {
			return nil, fmt.Errorf("data: %w", err)
		}
		return checkCount(gids, n)
	case "base64":
	default:
		return nil, fmt.Errorf("encoding %q: want \"csv\" or \"base64\"", encoding)
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("base64 data must be a string: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 data: %w", err)
	}
	switch compression {
	case "":
	case "zlib":
		zr, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("zlib data: %w", err)
		}
		if b, err = io.ReadAll(zr); err != nil {
			return nil, fmt.Errorf("zlib data: %w", err)
		}
	case "gzip":
		gr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("gzip data: %w", err)
		}
		if b, err = io.ReadAll(gr); err != nil {
			return nil, fmt.Errorf("gzip data: %w", err)
		}
	case "zstd":
		return nil, unsupported("compression", `"zstd" (use zlib, gzip or none)`)
	default:
		return nil, fmt.Errorf("compression %q: want \"zlib\", \"gzip\" or none", compression)
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("data is %d bytes, not a multiple of 4", len(b))
	}
	gids := make([]uint32, len(b)/4)
	for i := range gids {
		gids[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	return checkCount(gids, n)
}

func checkCount(gids []uint32, n int) ([]uint32, error) {
	if len(gids) != n {
		return nil, fmt.Errorf("data has %d cells, want %d", len(gids), n)
	}
	return gids, nil
}
