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
	"io"
	"strconv"
	"time"
)

type WARCHeader struct {
	url     string    // WARC-Target-URI
	ID      string    // WARC-Record-ID
	date    time.Time // WARC-Date
	size    int64     // Content-Length
	Type    string    // WARC-Type
	segment int       // WARC-Segment-Number
	fields  []byte
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
	rdr, err := newReader(r)
	if err != nil {
		return nil, err
	}
	return newWARCReader(rdr)
}

func newWARCReader(r *reader) (*WARCReader, error) {
	w := &WARCReader{&WARCHeader{}, r, nil}
	return w, w.reset()
}

func (w *WARCReader) Reset(r io.Reader) error {
	w.reader.reset(r)
	return w.reset()
}

func (w *WARCReader) reset() error {
	if v, err := w.peek(4); err != nil || string(v) != "WARC" {
		return ErrWARCHeader
	}
	return nil
}

func (w *WARCReader) Next() (Record, error) {
	// discard the returned slice as the first line in a WARC record is just the WARC header
	_, err := w.next()
	if err != nil {
		return nil, err
	}
	w.fields, err = w.storeLines(0, false)
	if err != nil {
		return nil, ErrWARCRecord
	}
	vals := getSelectValues(w.fields, "WARC-Type", "WARC-Target-URI", "WARC-Date", "Content-Length", "WARC-Record-ID", "WARC-Segment-Number")
	w.Type, w.url, w.ID = vals[0], vals[1], vals[4]
	w.date, err = time.Parse(time.RFC3339, vals[2])
	if err != nil {
		return nil, err
	}
	w.size, err = strconv.ParseInt(vals[3], 10, 64)
	if err != nil {
		return nil, err
	}
	w.thisIdx, w.sz = 0, w.Size()
	if vals[5] != "" {
		w.segment, err = strconv.Atoi(vals[5])
		if err != nil {
			return nil, err
		}
	} else {
		w.segment = 0
	}
	return w, nil
}

func (w *WARCReader) NextPayload() (Record, error) {
	for {
		r, err := w.Next()
		if err != nil {
			return r, err
		}
		if w.segment > 0 {
			if w.continuations == nil {
				w.continuations = make(continuations)
			}
			if c, ok := w.continuations.put(w); ok {
				return c, nil
			}
			continue
		}
		switch w.Type {
		default:
			continue
		case "resource", "conversion":
			return r, err
		case "response":
			if v, err := w.peek(5); err == nil && string(v) == "HTTP/" {
				l := len(w.fields)
				w.fields, err = w.storeLines(l, true)
			}
			return r, err
		}
	}
}
