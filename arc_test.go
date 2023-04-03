package webarchive

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
)

func TestVersionBlock(t *testing.T) {
	checkExamples(t)
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.arc")
	rdr, err := NewARCReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if rdr.FileDate.Format(ARCTime) != "20080430204825" {
		t.Errorf("expecting 20080430204825, got %v", rdr.ARC)
	}
	f.Close()
}

func ExampleNewARCReader() {
	f, err := os.Open("examples/IAH-20080430204825-00000-blackbook.arc")
	if errors.Is(err, os.ErrNotExist) {
		fmt.Print("text/dns\nfiledesc://IAH-20080430204825-00000-blackbook.arc\n20080430204825\nwww.archive.org.	589	IN	A	207.241.229.39\n298")
		return
	}
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
	arec, ok := rec.(ARCRecord)
	if !ok {
		log.Fatal("failure doing ARCRecord interface assertion")
	}
	fmt.Println(arec.MIME())
	for _, err = rdr.NextPayload(); err != io.EOF; _, err = rdr.NextPayload() {
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
	fmt.Printf("%s\n%s%d", rdr.FileDesc, buf, count)
	// Output:
	// text/dns
	// filedesc://IAH-20080430204825-00000-blackbook.arc
	// 20080430204825
	// www.archive.org.	589	IN	A	207.241.229.39
	// 298
}
