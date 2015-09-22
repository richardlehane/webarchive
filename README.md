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

For ARC, do:

    file, _ := os.Open("hello-world.arc")
    defer file.Close()
    doc, err := webarchive.NewReader(file)

For gzipped ARC or WARC, do:

    file, _ := os.Open("hello-world.warc.gz")
    defer file.Close()
    doc, err := webarchive.NewReader(file)

You can also iterate on Payload, rather than Record:

    record, err := doc.NextPayload()

This skips non-resource, conversion or response records and merges continuations into single records. It also strips HTTP headers from response records. After stripping, those HTTP headers are available alongside the WARC headers in the record.Fields() map.

Install with `go get github.com/richardlehane/webarchive`

[![Build Status](https://travis-ci.org/richardlehane/webarchive.png?branch=master)](https://travis-ci.org/richardlehane/webarchive)
