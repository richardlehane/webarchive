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
	idx     int64         // read index within the entire file
	thisIdx int64         // read index within the current record
	sz      int64         // size of the current record
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
	buf, err := r.Slice(r.idx, l)
	l = copy(p, buf)
	r.idx += int64(l)
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
	if r.thisIdx < r.sz {
		if r.slicer {
			r.idx += r.sz - r.thisIdx
		} else {
			discard(r.buf, int(r.sz-r.thisIdx))
		}
	}
	var slc []byte
	var err error
	// trim any leading blank lines, then return the first line with text
	// may reach io.EOF here in which case return that error for halting
	for slc, err = r.readLine(); err == nil && len(bytes.TrimSpace(slc)) == 0; slc, err = r.readLine() {
	}
	return slc, err
}

func (r *reader) readLine() ([]byte, error) {
	if r.slicer {
		l := 1024
		for {
			slc, err := r.src.(slicer).Slice(r.idx, l)
			i := bytes.IndexByte(slc, '\n')
			if i > -1 {
				r.idx += int64(i) + 1
				return slc[:i+1], nil
			}
			if err != nil {
				return nil, err
			}
			l += 1024
		}
	}
	return r.buf.ReadBytes('\n')
}

// read to first blank line and return a byte slice containing that content
// this is used to grab WARC and HTTP header blocks
func (r *reader) storeLines(i int) ([]byte, error) {
	if r.slicer {
		start := r.idx - int64(i)
		l := 1024
		for {
			slc, err := r.src.(slicer).Slice(r.idx, l)
			i := bytes.IndexByte(slc, '\n')
			if i > -1 {
				r.idx += int64(i) + 1
				if i < 3 {
					return r.src.(slicer).Slice(start, int(r.idx-start))
				}
			}
			if err != nil {
				return slc, err
			}
			l += 1024
		}
	}
	if r.store == nil {
		r.store = make([]byte, 4096)
	}
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
		return bytes.TrimSpace(buf), 0
	case nl == len(buf)-1:
		return bytes.TrimSpace(buf[:nl]), 0
	default:
		return bytes.TrimSpace(buf[:nl]), nl + 1
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

type continuations map[string]*continuation

func (c continuations) put(r Record) (Record, bool) {

	return nil, false
}

type continuation struct {
	*WARCHeader
	i   int
	buf [][]byte
}

func (c *continuation) size() int {
	var l int
	for _, b := range c.buf {
		l += len(b)
	}
	return l
}

func (c *continuation) index(i int) (int, int) {
	var tally int
	for idx, b := range c.buf {
		if i < tally+len(b) {
			return idx, i - tally
		}
		tally += len(b)
	}
	return len(c.buf) - 1, -1
}

func (c *continuation) Read(p []byte) (int, error) {
	_, o := c.index(c.i)
	if o < 0 {
		return 0, io.EOF
	}
	l := len(p)
	if l > c.size()-c.i {
		l = c.size() - c.i
	}
	return l, nil
}

func (c *continuation) Slice(off int64, l int) ([]byte, error) {

	return nil, nil
}

func (c *continuation) EofSlice(off int64, l int) ([]byte, error) {
	return nil, nil
}
