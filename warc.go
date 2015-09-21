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
	"strings"
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
	w.fields, err = w.storeLines()
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
	return w.Next()
}

func getLines(buf []byte) func() []byte {
	readline := func() ([]byte, int) {
		nl := bytes.IndexByte(buf, '\n')
		switch {
		case nl < 0:
			return bytes.TrimSpace(buf), 0
		case nl == len(buf)-1:
			return bytes.TrimSpace(buf[:nl]), 0
		default:
			return bytes.TrimSpace(buf[:nl]), nl + 1
		}
	}
	skipspace := func() int {
		n := 0
		for {
			if n == len(buf) {
				return n
			}
			c := buf[n]
			if c != ' ' && c != '\t' {
				return n
			}
			n++
		}
	}
	return func() []byte {
		if buf == nil {
			return nil
		}
		ret, adv := readline()
		if adv == 0 {
			buf = nil
			return ret
		}
		buf = buf[adv:]
		for s := skipspace(); s > 0; s = skipspace() {
			buf = buf[s:]
			n, a := readline()
			ret = append(append(ret, ' '), n...)
			if a == 0 {
				buf = nil
				return ret
			}
			buf = buf[a:]
		}
		return ret
	}
}

func normaliseKey(k []byte) string {
	parts := bytes.Split(k, []byte("-"))
	for i, v := range parts {
		parts[i] = []byte(strings.Title(string(v)))
	}
	return string(bytes.Join(parts, []byte("-")))
}

func getSelectValues(buf []byte, vals ...string) []string {
	ret := make([]string, len(vals))
	lines := getLines(buf)
	for l := lines(); l != nil; l = lines() {
		parts := bytes.SplitN(l, []byte(":"), 2)
		if len(parts) == 2 {
			k := normaliseKey(parts[0])
			for i, s := range vals {
				if s == k {
					ret[i] = string(bytes.TrimSpace(parts[1]))
				}
			}
		}
	}
	return ret
}

func getAllValues(buf []byte) map[string][]string {
	ret := make(map[string][]string)
	lines := getLines(buf)
	for l := lines(); l != nil; l = lines() {
		parts := bytes.Split(l, []byte(":"))
		if len(parts) == 2 {
			k := normaliseKey(parts[0])
			ret[k] = append(ret[k], string(bytes.TrimSpace(parts[1])))
		}
	}
	return ret
}
