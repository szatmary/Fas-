package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const chunkSize = 32

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <output.db>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Reads YUV4MPEG2 (C444) from stdin, chunks into 32x32 blocks.\n")
		fmt.Fprintf(os.Stderr, "\nexample:\n")
		fmt.Fprintf(os.Stderr, "  ffmpeg -i video.mp4 -pix_fmt yuv444p -f yuv4mpegpipe - | %s chunks.db\n", os.Args[0])
		os.Exit(1)
	}

	dbPath := os.Args[1]
	if err := ingest(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func ingest(dbPath string) error {
	reader := bufio.NewReaderSize(os.Stdin, 8*1024*1024)

	// Parse Y4M stream header
	width, height, err := parseY4MHeader(reader)
	if err != nil {
		return fmt.Errorf("parsing Y4M header: %w", err)
	}
	fmt.Printf("Y4M stream: %dx%d\n", width, height)

	// Compute chunk grid
	paddedW := ((width + chunkSize - 1) / chunkSize) * chunkSize
	paddedH := ((height + chunkSize - 1) / chunkSize) * chunkSize
	chunksX := paddedW / chunkSize
	chunksY := paddedH / chunkSize
	fmt.Printf("Padded: %dx%d (%dx%d chunks per plane)\n", paddedW, paddedH, chunksX, chunksY)

	// Set up database
	os.Remove(dbPath)
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=OFF")
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}

	// Store metadata
	for _, kv := range [][2]string{
		{"orig_width", fmt.Sprintf("%d", width)},
		{"orig_height", fmt.Sprintf("%d", height)},
		{"padded_width", fmt.Sprintf("%d", paddedW)},
		{"padded_height", fmt.Sprintf("%d", paddedH)},
		{"chunk_size", fmt.Sprintf("%d", chunkSize)},
	} {
		if _, err := db.Exec("INSERT INTO metadata (key, value) VALUES (?, ?)", kv[0], kv[1]); err != nil {
			return err
		}
	}

	// Read frames and chunk them
	planeSize := width * height
	framePixels := planeSize * 3 // YUV444: all planes same size
	frameBuf := make([]byte, framePixels)
	paddedPlane := make([]byte, paddedW*paddedH)
	chunkBuf := make([]byte, chunkSize*chunkSize)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmts, err := prepareStmts(tx)
	if err != nil {
		return err
	}

	planeNames := []string{"Y", "U", "V"}
	const batchSize = 10000
	insertCount := 0
	totalBlocks := 0
	dedupBlocks := 0
	frame := 0

	for {
		// Each frame starts with "FRAME\n" (possibly with parameters after FRAME)
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading frame header at frame %d: %w", frame, err)
		}
		if !strings.HasPrefix(line, "FRAME") {
			return fmt.Errorf("expected FRAME header at frame %d, got %q", frame, line)
		}

		// Read raw YUV444 pixel data
		if _, err := io.ReadFull(reader, frameBuf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return fmt.Errorf("reading frame %d pixel data: %w", frame, err)
		}

		for p, planeName := range planeNames {
			planeData := frameBuf[p*planeSize : (p+1)*planeSize]
			padPlane(paddedPlane, planeData, width, height, paddedW, paddedH)

			for cy := 0; cy < chunksY; cy++ {
				for cx := 0; cx < chunksX; cx++ {
					extractChunk(chunkBuf, paddedPlane, paddedW, cx, cy, chunkSize)

					h := sha256.Sum256(chunkBuf)
					hash := h[:]

					var blockID int64
					err := stmts.findBlock.QueryRow(hash).Scan(&blockID)
					if err == sql.ErrNoRows {
						res, err := stmts.insertBlock.Exec(hash, append([]byte(nil), chunkBuf...))
						if err != nil {
							return fmt.Errorf("inserting block f=%d p=%s cx=%d cy=%d: %w", frame, planeName, cx, cy, err)
						}
						blockID, _ = res.LastInsertId()
						totalBlocks++
					} else if err != nil {
						return fmt.Errorf("finding block: %w", err)
					} else {
						if _, err := stmts.incrRef.Exec(blockID); err != nil {
							return fmt.Errorf("incrementing ref_count: %w", err)
						}
						dedupBlocks++
					}

					if _, err := stmts.insertChunk.Exec(frame, planeName, cx, cy, blockID); err != nil {
						return fmt.Errorf("inserting chunk f=%d p=%s cx=%d cy=%d: %w", frame, planeName, cx, cy, err)
					}

					insertCount++
					if insertCount%batchSize == 0 {
						if err := tx.Commit(); err != nil {
							return err
						}
						tx, err = db.Begin()
						if err != nil {
							return err
						}
						stmts, err = prepareStmts(tx)
						if err != nil {
							return err
						}
					}
				}
			}
		}

		frame++
		if frame%30 == 0 {
			fmt.Printf("\rProcessed %d frames  (unique blocks: %d, duplicates: %d)", frame, totalBlocks, dedupBlocks)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Update frame count in metadata
	if _, err := db.Exec("INSERT INTO metadata (key, value) VALUES (?, ?)", "frame_count", fmt.Sprintf("%d", frame)); err != nil {
		return err
	}

	fmt.Printf("\rProcessed %d frames  (unique blocks: %d, duplicates: %d)\n", frame, totalBlocks, dedupBlocks)
	fmt.Printf("Total chunk references: %d\n", insertCount)
	fmt.Printf("Unique blocks stored:   %d\n", totalBlocks)
	fmt.Printf("Duplicate references:   %d\n", dedupBlocks)

	return nil
}

// parseY4MHeader reads the YUV4MPEG2 header line and returns width, height.
// Expects the colorspace to be C444 (yuv444p).
func parseY4MHeader(r *bufio.Reader) (width, height int, err error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, 0, fmt.Errorf("reading header: %w", err)
	}
	line = strings.TrimRight(line, "\n")

	if !strings.HasPrefix(line, "YUV4MPEG2") {
		return 0, 0, fmt.Errorf("not a YUV4MPEG2 stream: %q", line)
	}

	parts := strings.Fields(line)
	for _, p := range parts[1:] {
		if len(p) < 2 {
			continue
		}
		switch p[0] {
		case 'W':
			width, err = strconv.Atoi(p[1:])
			if err != nil {
				return 0, 0, fmt.Errorf("parsing width %q: %w", p, err)
			}
		case 'H':
			height, err = strconv.Atoi(p[1:])
			if err != nil {
				return 0, 0, fmt.Errorf("parsing height %q: %w", p, err)
			}
		case 'C':
			if p[1:] != "444" {
				return 0, 0, fmt.Errorf("unsupported colorspace %q, need C444 (use ffmpeg -pix_fmt yuv444p)", p)
			}
		}
	}

	if width == 0 || height == 0 {
		return 0, 0, fmt.Errorf("missing width/height in header: %q", line)
	}
	return width, height, nil
}

type stmtSet struct {
	insertBlock *sql.Stmt
	incrRef     *sql.Stmt
	findBlock   *sql.Stmt
	insertChunk *sql.Stmt
}

func prepareStmts(tx *sql.Tx) (stmtSet, error) {
	var s stmtSet
	var err error
	s.insertBlock, err = tx.Prepare("INSERT INTO blocks (hash, data) VALUES (?, ?)")
	if err != nil {
		return s, err
	}
	s.incrRef, err = tx.Prepare("UPDATE blocks SET ref_count = ref_count + 1 WHERE id = ?")
	if err != nil {
		return s, err
	}
	s.findBlock, err = tx.Prepare("SELECT id FROM blocks WHERE hash = ?")
	if err != nil {
		return s, err
	}
	s.insertChunk, err = tx.Prepare("INSERT INTO chunks (frame, plane, chunk_x, chunk_y, block_id) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return s, err
	}
	return s, nil
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE metadata (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE blocks (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			hash      BLOB    NOT NULL UNIQUE,
			data      BLOB    NOT NULL,
			ref_count INTEGER NOT NULL DEFAULT 1
		);

		CREATE TABLE chunks (
			frame    INTEGER NOT NULL,
			plane    TEXT    NOT NULL CHECK(plane IN ('Y','U','V')),
			chunk_x  INTEGER NOT NULL,
			chunk_y  INTEGER NOT NULL,
			block_id INTEGER NOT NULL REFERENCES blocks(id),
			PRIMARY KEY (frame, plane, chunk_x, chunk_y)
		);
	`)
	return err
}

func padPlane(dst, src []byte, origW, origH, padW, padH int) {
	for y := 0; y < padH; y++ {
		srcY := y
		if srcY >= origH {
			srcY = origH - 1
		}
		for x := 0; x < padW; x++ {
			srcX := x
			if srcX >= origW {
				srcX = origW - 1
			}
			dst[y*padW+x] = src[srcY*origW+srcX]
		}
	}
}

func extractChunk(dst, plane []byte, planeW, cx, cy, cs int) {
	startX := cx * cs
	startY := cy * cs
	for row := 0; row < cs; row++ {
		copy(dst[row*cs:(row+1)*cs], plane[(startY+row)*planeW+startX:])
	}
}
