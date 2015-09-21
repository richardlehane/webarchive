package webarchive

import (
	"fmt"
	"io"
	"log"
	"os"
	"testing"
	"time"
)

func TestWARC(t *testing.T) {
	f, _ := os.Open("examples/hello-world.warc")
	defer f.Close()
	rdr, err := NewWARCReader(f)
	if err != nil {
		t.Fatal("failure loading example: " + err.Error())
	}
	rec, err := rdr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Date().Format(time.RFC3339) != "2015-07-08T21:55:13Z" {
		t.Errorf("expecting 2015-07-08T21:55:13Z, got %v", rec.Date())
	}
}

func ExampleBlackbookWARC() {
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.warc")
	rdr, err := NewWARCReader(f)
	if err != nil {
		log.Fatal("failure creating an warc reader")
	}
	rec, err := rdr.Next()
	if err != nil {
		log.Fatal("failure seeking: " + err.Error())
	}
	buf := make([]byte, 55)
	io.ReadFull(rec, buf)
	var count int
	for _, err = rdr.Next(); err != io.EOF; _, err = rdr.Next() {
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
	fmt.Printf("%s\n%d", buf, count)
	// Output:
	// software: Heritrix/@VERSION@ http://crawler.archive.org
	// 821
}
