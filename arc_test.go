package webarchive

import (
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"github.com/richardlehane/siegfried/pkg/core/siegreader"
)

func TestVersionBlock(t *testing.T) {
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.arc")
	rdr, err := NewARCReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if rdr.FileDate.Format(ARCTime) != "20080430204825" {
		t.Errorf("expecting 20080430204825, got %v", rdr.ARC)
	}
	f.Close()
	f, _ = os.Open("examples/hello-world2.arc")
	defer f.Close()
	buffers := siegreader.New()
	buf, _ := buffers.Get(f)
	rdr, err = NewARCReader(siegreader.ReaderFrom(buf))
	if err != nil {
		t.Fatal(err)
	}
	if rdr.FileDate.Format(ARCTime) != "19960923142103" {
		t.Errorf("expecting 19960923142103, got %v", rdr.ARC)
	}
}

func ExampleBlackbookARC() {
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.arc")
	rdr, err := NewARCReader(f)
	if err != nil {
		log.Fatal("failure creating an arc reader")
	}
	rec, err := rdr.NextPayload()
	if err != nil {
		log.Fatal("failure seeking")
	}
	buf := make([]byte, 56)
	io.ReadFull(rec, buf)
	var count int
	for _, err = rdr.NextPayload(); err != io.EOF; _, err = rdr.NextPayload() {
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
	fmt.Printf("%s\n%s%d", rdr.Path, buf, count)
	// Output:
	// filedesc://IAH-20080430204825-00000-blackbook.arc
	// 20080430204825
	// www.archive.org.	589	IN	A	207.241.229.39
	// 298
}
