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
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"strings"
)

// siegfried related: siegfried buffers have a slice method, the use here allows re-use of that underlying buffer
// siegfried is https://github.com/richardlehane/siegfried
type slicer interface {
	Slice(off int64, l int) ([]byte, error)
}

type reader struct {
	src     io.Reader     // reference to the provided reader
	sbuf    *bufio.Reader // buffer src if not a slicer
	buf     *bufio.Reader // buf will point to sbuf, unless src is gzip
	closer  io.ReadCloser // if gzip, hold reference to close it
	slicer  bool          // does the source conform to the slicer interface? (siegfried related: siegfried buffers have this method)
	idx     int64         // read index within the entire file - stays at the start of the Record/Payload until Next is called
	thisIdx int64         // read index within the current record
	sz      int64         // size of the current record (Read area)
	store   []byte        // used as temp store for fields
}

func (r *reader) Read(p []byte) (int, error) {
	if r.thisIdx >= r.sz {
		return 0, io.EOF
	}
	l := len(p)
	if int64(len(p)) > r.sz-r.thisIdx {
		l = int(r.sz - r.thisIdx)
	}
	r.thisIdx += int64(l)
	if !r.slicer {
		return fullRead(r.buf, p[:l])
	}
	buf, err := r.src.(slicer).Slice(r.idx+r.thisIdx-int64(l), l)
	l = copy(p, buf)
	return l, err
}

func (r *reader) Slice(off int64, l int) ([]byte, error) {
	if !r.slicer {
		return nil, ErrNotSlicer
	}
	if off >= r.sz {
		return nil, io.EOF
	}
	var err error
	if l > int(r.sz-off) {
		l, err = int(r.sz-off), io.EOF
	}
	slc, err1 := r.src.(slicer).Slice(r.idx+off, l)
	if err1 != nil {
		err = err1
	}
	return slc, err
}

func (r *reader) EofSlice(off int64, l int) ([]byte, error) {
	if !r.slicer {
		return nil, ErrNotSlicer
	}
	if off >= r.sz {
		return nil, io.EOF
	}
	var err error
	if l > int(r.sz-off) {
		l, off, err = int(r.sz-off), 0, io.EOF
	} else {
		off = r.sz - off - int64(l)
	}
	slc, err1 := r.src.(slicer).Slice(r.idx+off, l)
	if err1 != nil {
		err = err1
	}
	return slc, err
}

func (r *reader) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

func newReader(s io.Reader) (*reader, error) {
	r := &reader{src: s}
	if _, ok := s.(slicer); ok {
		r.slicer = true
	} else {
		r.sbuf = bufio.NewReader(s)
	}
	err := r.unzip()
	return r, err
}

func (r *reader) reset(s io.Reader) error {
	r.src = s
	if _, ok := s.(slicer); ok {
		r.slicer = true
	} else {
		r.slicer = false
		if r.sbuf == nil {
			r.sbuf = bufio.NewReader(s)
		} else {
			r.sbuf.Reset(s)
		}
	}
	r.idx, r.thisIdx, r.sz = 0, 0, 0
	return r.unzip()
}

func isgzip(buf []byte) bool {
	if buf[0] != 0x1f || buf[1] != 0x8b || buf[2] != 8 {
		return false
	}
	return true
}

func (r *reader) unzip() error {
	if buf, err := r.srcpeek(3); err == nil && isgzip(buf) {
		var gr *gzip.Reader
		if r.slicer {
			gr, err = gzip.NewReader(r.src)
		} else {
			gr, err = gzip.NewReader(r.sbuf)
		}
		if err != nil {
			return err
		}
		r.closer = gr
		if r.buf == nil || r.buf == r.sbuf {
			r.buf = bufio.NewReader(gr)
		} else {
			r.buf.Reset(gr)
		}
		r.slicer = false
	} else {
		r.closer = nil
		r.buf = r.sbuf
	}
	return nil
}

// peek from r.src (rather than usual r.buf)
func (r *reader) srcpeek(i int) ([]byte, error) {
	if r.slicer {
		return r.src.(slicer).Slice(r.idx, i)
	}
	return r.sbuf.Peek(i)
}

func (r *reader) peek(i int) ([]byte, error) {
	if r.slicer {
		return r.src.(slicer).Slice(r.idx, i)
	}
	return r.buf.Peek(i)
}

func (r *reader) next() ([]byte, error) {
	// advance if haven't read the previous record
	r.idx += r.sz
	if r.thisIdx < r.sz && !r.slicer {
		discard(r.buf, int(r.sz-r.thisIdx))
	}
	var slc []byte
	var err error
	// trim any leading blank lines, then return the first line with text
	// may reach io.EOF here in which case return that error for halting
	for slc, err = r.readLine(); err == nil && len(bytes.TrimSpace(slc)) == 0; slc, err = r.readLine() {
	}
	return slc, err
}

// if a slicer - advance r.idx
func (r *reader) readLine() ([]byte, error) {
	if r.slicer {
		l := 100
		for {
			slc, err := r.src.(slicer).Slice(r.idx, l)
			i := bytes.IndexByte(slc, '\n')
			if i > -1 {
				r.idx += int64(i) + 1
				return slc[:i+1], nil
			}
			if err != nil || len(slc) < l {
				if err == nil {
					err = io.EOF
				}
				return nil, err
			}
			l += 100
		}
	}
	return r.buf.ReadBytes('\n')
}

func indexBlankLine(buf []byte) int {
	var i int
	for {
		idx := bytes.IndexByte(buf[i:], '\n')
		if idx > -1 {
			i += idx + 1
			if idx < 3 {
				return i
			}
		} else {
			return -1
		}
	}
}

// read to first blank line and return a byte slice containing that content
// this is used to grab WARC and HTTP header blocks
func (r *reader) storeLines(i int, alter bool) ([]byte, error) {
	if r.slicer {
		start := r.idx - int64(i)
		l := 1000
		for {
			slc, err := r.src.(slicer).Slice(r.idx, l)
			if len(slc) == 0 {
				return nil, err
			}
			idx := indexBlankLine(slc)
			if idx > -1 {
				r.idx += int64(idx)
				if alter {
					r.sz -= int64(idx)
				}
				return r.src.(slicer).Slice(start, int(r.idx-start))
			}
			if len(slc) < l {
				return nil, io.EOF
			}
			l += 1000
		}
	}
	if r.store == nil {
		r.store = make([]byte, 4096)
	}
	alterSz := i
	for {
		slc, err := r.buf.ReadBytes('\n')
		if err != nil {
			return r.store[:i], err
		}
		if len(slc)+i < len(r.store) {
			copy(r.store[i:], slc)
		} else {
			nb := make([]byte, len(r.store)+len(slc))
			copy(nb, r.store)
			copy(nb[i:], slc)
			r.store = nb
		}
		i += len(slc)
		if len(slc) < 3 {
			if alter {
				r.sz -= int64(i - alterSz)
			}
			return r.store[:i], err
		}
	}
}

func fullRead(r *bufio.Reader, p []byte) (int, error) {
	var idx int
	for {
		i, err := r.Read(p[idx:])
		idx += i
		if err != nil || idx >= len(p) {
			return idx, err
		}
	}
}

func readline(buf []byte) ([]byte, int) {
	nl := bytes.IndexByte(buf, '\n')
	switch {
	case nl < 0:
		return buf, 0
	case nl == len(buf)-1:
		return buf[:nl], 0
	default:
		return buf[:nl], nl + 1
	}
}

func skipspace(buf []byte) int {
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

// function that iterates through a byte slice, returning each individual line
func getLines(buf []byte) func() []byte {
	return func() []byte {
		if buf == nil {
			return nil
		}
		ret, adv := readline(buf)
		if adv == 0 {
			buf = nil
			return ret
		}
		buf = buf[adv:]
		for s := skipspace(buf); s > 0; s = skipspace(buf) {
			buf = buf[s:]
			n, a := readline(buf)
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

var WARCHeaders = map[string]string{
	"Warc-Type":                    "WARC-Type",
	"Warc-Record-Id":               "WARC-Record-ID",
	"Warc-Date":                    "WARC-Date",
	"Content-Length":               "Content-Length",
	"Content-Type":                 "Content-Type",
	"Warc-Concurrent-To":           "WARC-Concurrent-To",
	"Warc-Block-Digest":            "WARC-Block-Digest",
	"Warc-Payload-Digest":          "WARC-Payload-Digest",
	"Warc-Ip-Address":              "WARC-IP-Address",
	"Warc-Refers-To":               "WARC-Refers-To",
	"Warc-Target-Uri":              "WARC-Target-URI",
	"Warc-Truncated":               "WARC-Truncated",
	"Warc-Warcinfo-Id":             "WARC-Warcinfo-ID",
	"Warc-Filename":                "WARC-Filename",
	"Warc-Profile":                 "WARC-Profile",
	"Warc-Identified-Payload-Type": "WARC-Identified-Payload-Type",
	"Warc-Segment-Origin-Id":       "WARC-Segment-Origin-ID",
	"Warc-Segment-Number":          "WARC-Segment-Number",
	"Warc-Segment-Total-Length":    "WARC-Segment-Total-Length",
}

func normaliseKey(k []byte) string {
	parts := bytes.Split(k, []byte("-"))
	for i, v := range parts {
		parts[i] = []byte(strings.Title(strings.ToLower(string(v))))
	}
	s := string(bytes.Join(parts, []byte("-")))
	if w := WARCHeaders[s]; w != "" {
		return w
	}
	return s
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

type continuations map[string]*continuation

func (c continuations) put(w *WARCReader) (Record, bool) {
	var id string
	var final bool
	if w.WARCHeader.segment > 1 {
		fields := w.WARCHeader.Fields()
		s, ok := fields["WARC-Segment-Origin-ID"]
		if ok {
			id = s[0]
		}
		_, final = fields["WARC-Segment-Total-Length"] // if we have this field, can mark continuation as complete
	} else {
		id = w.WARCHeader.ID
	}
	cr, ok := c[id]
	if !ok {
		cr = &continuation{
			WARCHeader: &WARCHeader{
				url:    w.WARCHeader.url,
				ID:     w.WARCHeader.ID,
				date:   w.WARCHeader.date,
				Type:   w.WARCHeader.Type,
				fields: make([]byte, len(w.WARCHeader.fields)),
			},
			bufs: make([][]byte, w.WARCHeader.segment),
		}
		copy(cr.WARCHeader.fields, w.WARCHeader.fields)
		c[id] = cr
	}
	if final {
		cr.final = true
	}
	if len(cr.bufs) < w.WARCHeader.segment {
		nb := make([][]byte, w.WARCHeader.segment)
		copy(nb, cr.bufs)
	}
	cr.bufs[w.WARCHeader.segment-1], _ = ioutil.ReadAll(w)
	if !cr.complete() {
		return nil, false
	}
	delete(c, id) // clear the continutation before returning
	return cr, true
}

type continuation struct {
	*WARCHeader
	final bool
	idx   int
	start int
	bufs  [][]byte
	buf   []byte
}

// check completeness - have final segment and all previous segments
func (c *continuation) complete() bool {
	if !c.final {
		return false
	}
	var sz int
	for _, b := range c.bufs {
		if b == nil {
			return false
		}
		sz += len(b)
	}
	c.buf = make([]byte, sz+len(c.fields))
	idx := len(c.fields)
	copy(c.buf[:idx], c.fields)
	for _, b := range c.bufs {
		copy(c.buf[idx:], b)
		idx += len(b)
	}
	c.idx, c.start = len(c.fields), len(c.fields)
	c.fields = c.buf[:c.idx]
	if len(c.buf[c.idx:]) > 5 && string(c.buf[c.idx:c.idx+5]) == "HTTP/" {
		bi := indexBlankLine(c.buf[c.idx:])
		if bi > -1 {
			c.idx += bi
			c.start += bi
			c.fields = c.buf[:c.idx]
		}
	}
	return true
}

func (c *continuation) Read(p []byte) (int, error) {
	if c.idx >= len(c.buf) {
		return 0, io.EOF
	}
	var err error
	l := len(p)
	if l > len(c.buf)-c.idx {
		l = len(c.buf) - c.idx
		err = io.EOF
	}
	copy(p, c.buf[c.idx:l])
	c.idx += l
	return l, err
}

func (c *continuation) Slice(off int64, l int) ([]byte, error) {
	if c.start+int(off) >= len(c.buf) {
		return nil, io.EOF
	}
	var err error
	if l > len(c.buf)-c.start-int(off) {
		l, err = len(c.buf)-c.start-int(off), io.EOF
	}
	return c.buf[c.start+int(off) : c.start+int(off)+l], err
}

func (c *continuation) EofSlice(off int64, l int) ([]byte, error) {
	if int(off)+c.start >= len(c.buf) {
		return nil, io.EOF
	}
	var o int
	var err error
	if l > len(c.buf)-c.start-int(off) {
		l, o, err = len(c.buf)-c.start-int(off), 0, io.EOF
	} else {
		o = len(c.buf) - c.start - int(off) - l
	}
	return c.buf[o:l], err
}
