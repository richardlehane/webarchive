package webarchive

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/richardlehane/siegfried/pkg/core/siegreader"
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

func TestReaders(t *testing.T) {
	buf, _ := ioutil.ReadFile("examples/hello-world.warc")
	rdr := bytes.NewReader(buf)
	rdr2 := bytes.NewReader(buf)
	buffers := siegreader.New()
	sbuf, _ := buffers.Get(rdr)
	wrdr, _ := NewWARCReader(siegreader.ReaderFrom(sbuf))
	wrdr2, _ := NewWARCReader(rdr2)
	var count int
	for {
		count++
		r1, err1 := wrdr.Next()
		r2, err2 := wrdr2.Next()
		if err1 != err2 {
			t.Fatalf("unequal errors %v, %v, %d", err1, err2, count)
		}
		if err1 != nil {
			break
		}
		if r1.URL() != r2.URL() {
			t.Fatalf("unequal urls, %s, %s, %d", r1.URL(), r2.URL(), count)
		}
		b1, _ := ioutil.ReadAll(r1)
		b2, _ := ioutil.ReadAll(r2)
		if !bytes.Equal(b1, b2) {
			t.Fatalf("reads aren't equal at %d:\nfirst read:\n%s\n\nsecond read:\n%s\n\n", count, string(b1), string(b2))
		}
	}
}

func TestGZ(t *testing.T) {
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.warc.gz")
	defer f.Close()
	rdr, err := NewWARCReader(f)
	if err != nil {
		t.Fatal("failure loading example: " + err.Error())
	}
	defer rdr.Close()
	var count int
	for _, err = rdr.NextPayload(); err != io.EOF; _, err = rdr.NextPayload() {
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
	if count != 299 {
		t.Errorf("expecting 299 payloads, got %d", count)
	}
}

func ExampleBlackbookWARC() {
	f, _ := os.Open("examples/IAH-20080430204825-00000-blackbook.warc")
	rdr, err := NewWARCReader(f)
	if err != nil {
		log.Fatal("failure creating an warc reader")
	}
	rec, err := rdr.NextPayload()
	if err != nil {
		log.Fatal("failure seeking: " + err.Error())
	}
	buf := make([]byte, 55)
	io.ReadFull(rec, buf)
	var count int
	for _, err = rdr.NextPayload(); err != io.EOF; _, err = rdr.NextPayload() {
		if err != nil {
			log.Fatal(err)
		}
		count++
	}
	fmt.Printf("%s\n%d", buf, count)
	// Output:
	// 20080430204825
	// www.archive.org.	589	IN	A	207.241.229.39
	// 298
}
