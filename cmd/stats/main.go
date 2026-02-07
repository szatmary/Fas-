package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <db>\n", os.Args[0])
		os.Exit(1)
	}
	db, _ := sql.Open("sqlite3", os.Args[1]+"?mode=ro")
	defer db.Close()

	var uniqueBlocks, totalRefs int
	var dupeRefs int
	db.QueryRow("SELECT COUNT(*), SUM(ref_count) FROM blocks").Scan(&uniqueBlocks, &totalRefs)
	db.QueryRow("SELECT SUM(ref_count - 1) FROM blocks WHERE ref_count > 1").Scan(&dupeRefs)

	var chunkRows int
	db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&chunkRows)

	var frames int
	db.QueryRow("SELECT value FROM metadata WHERE key='frame_count'").Scan(&frames)

	var dbSize int64
	db.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&dbSize)

	fmt.Printf("Frames:                 %d\n", frames)
	fmt.Printf("Chunk rows:             %d\n", chunkRows)
	fmt.Printf("Block references (3/chunk): %d\n", totalRefs)
	fmt.Printf("Unique blocks:          %d\n", uniqueBlocks)
	fmt.Printf("Duplicate refs:         %d\n", dupeRefs)
	fmt.Printf("Dedup ratio:            %.2f%%\n", float64(dupeRefs)/float64(totalRefs)*100)
	var avgLen float64
	db.QueryRow("SELECT AVG(LENGTH(data)) FROM blocks").Scan(&avgLen)
	fmt.Printf("Block data size:        %.1f MB (avg %.0f bytes each)\n", float64(uniqueBlocks)*avgLen/1e6, avgLen)
	fmt.Printf("DB file size:           %.1f GB\n", float64(dbSize)/1e9)
	fmt.Println()

	fmt.Println("Top 10 most-reused blocks:")
	rows, _ := db.Query("SELECT rowid, ref_count FROM blocks ORDER BY ref_count DESC LIMIT 10")
	for rows.Next() {
		var id, rc int
		rows.Scan(&id, &rc)
		fmt.Printf("  rowid %-10d  refs: %d\n", id, rc)
	}
	rows.Close()

	fmt.Println()
	fmt.Println("Ref count distribution:")
	dist, _ := db.Query(`
		SELECT ref_count, COUNT(*) as cnt
		FROM blocks
		GROUP BY ref_count
		ORDER BY ref_count
		LIMIT 20`)
	for dist.Next() {
		var rc, cnt int
		dist.Scan(&rc, &cnt)
		fmt.Printf("  ref_count=%d: %d blocks\n", rc, cnt)
	}
	dist.Close()
}
