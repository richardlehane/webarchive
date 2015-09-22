Warning: *pre-production*!

A reader for the WARC and ARC web archive formats.

This package has been written for use in [https://github.com/richardlehane/siegfried](https://github.com/richardlehane/siegfried) and has a bunch of quirks relating to that specific use case. If you're after a general purpose golang warc package, you might be better suited by one of these excellent choices:

  - [https://github.com/edsu/warc](https://github.com/edsu/warc)
  - [https://github.com/slyrz/warc](https://github.com/slyrz/warc)

Example usage:

    file, _ := os.Open("hello-world.warc")
    defer file.Close()
    doc, err := webarchive.NewReader(file)
    if err != nil {
      log.Fatal(err)
    }
    for record, err := doc.Next(); err == nil; entry, err = doc.Next() {
      buf := make([]byte, 512)
      i, _ := doc.Read(buf)
      if i > 0 {
        fmt.Println(buf[:i])
      }
      fmt.Println(record.URL())
    }

Install with `go get github.com/richardlehane/webarchive`

[![Build Status](https://travis-ci.org/richardlehane/webarchive.png?branch=master)](https://travis-ci.org/richardlehane/webarchive)
