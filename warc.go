// Copyright 2015 Richard Lehane. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webarchive

import (
	"bytes"
	"io"
	"strconv"
	"time"
)

type WARCHeader struct {
	url    string    // WARC-Target-URI
	ID     string    // WARC-Record-ID
	date   time.Time // WARC-Date
	size   int64     // Content-Length
	Type   string    // WARC-Type
	fields []byte
}

func (h *WARCHeader) URL() string                 { return h.url }
func (h *WARCHeader) Date() time.Time             { return h.date }
func (h *WARCHeader) Size() int64                 { return h.size }
func (h *WARCHeader) Fields() map[string][]string { return getAllValues(h.fields) }

type WARCReader struct {
	*WARCHeader
	*reader
	continuations
}

func NewWARCReader(r io.Reader) (*WARCReader, error) {
	rdr := newReader(r)
	v, err := rdr.peek(4)
	if err != nil {
		return nil, err
	}
	if string(v) != "WARC" {
		return nil, ErrWARCHeader
	}
	return &WARCReader{&WARCHeader{}, rdr, nil}, nil
}

func (w *WARCReader) Reset(r io.Reader) error {
	w.reader.reset(r)
	return nil
}

func (w *WARCReader) Next() (Record, error) {
	// advance if haven't read the previous record
	if w.thisIdx < w.sz {
		if w.slicer {
			w.idx += w.sz - w.thisIdx
		} else {
			discard(w.buf, int(w.sz-w.thisIdx))
		}
	}
	var slc []byte
	var err error
	for slc, err = w.readLine(); err == nil && len(bytes.TrimSpace(slc)) == 0; slc, err = w.readLine() {
	}
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, ErrWARCHeader
	}
	w.fields, err = w.storeLines(0)
	if err != nil {
		return nil, ErrWARCRecord
	}
	vals := getSelectValues(w.fields, "WARC-Type", "WARC-Target-URI", "WARC-Date", "Content-Length", "WARC-Record-ID")
	date, err := time.Parse(time.RFC3339, vals[2])
	if err != nil {
		return nil, err
	}
	sz, err := strconv.ParseInt(vals[3], 10, 64)
	if err != nil {
		return nil, err
	}
	w.Type, w.url, w.date, w.size, w.ID = vals[0], vals[1], date, sz, vals[4]
	w.thisIdx, w.sz = 0, w.Size()
	return w, nil
}

func (w *WARCReader) NextPayload() (Record, error) {
	for {
		r, err := w.Next()
		if err != nil {
			return r, err
		}
		switch w.Type {
		default:
			continue
		case "resource", "conversion":
			return r, err
			//case "continuation":
			//	if cr, ok := w.continuations.put(r); ok {
			//		return cr, nil
			//	}
		case "response":
			if v, err := w.peek(5); err == nil && string(v) == "HTTP/" {
				l := len(w.fields)
				w.fields, err = w.storeLines(l)
				w.thisIdx += int64(len(w.fields) - l)
			}
			return r, err
		}
	}
}
