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
	"io"
)

type reader struct {
	src     io.Reader
	buf     *bufio.Reader
	slicer  bool
	idx     int64
	thisIdx int64
	sz      int64
	store   []byte
}

type slicer interface {
	Slice(off int64, l int) ([]byte, error)
}

func newReader(r io.Reader) *reader {
	rdr := &reader{src: r}
	if _, ok := r.(slicer); ok {
		rdr.slicer = true
	} else {
		rdr.buf = bufio.NewReader(r)
	}
	return rdr
}

func (r *reader) reset(s io.Reader) {
	r.src = s
	if _, ok := s.(slicer); ok {
		r.slicer = true
	} else {
		r.slicer = false
		r.buf.Reset(s)
	}
	r.idx, r.thisIdx, r.sz = 0, 0, 0
}

func (r *reader) peek(i int) ([]byte, error) {
	if r.slicer {
		return r.src.(slicer).Slice(r.idx, i)
	}
	return r.buf.Peek(i)
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

// read to first blank line
func (r *reader) storeLines() ([]byte, error) {
	if r.slicer {
		start := r.idx
		l := 1024
		for {
			slc, err := r.src.(slicer).Slice(r.idx, l)
			i := bytes.IndexByte(slc, '\n')
			if i > -1 {
				r.idx += int64(i) + 1
				if i < 3 {
					return r.src.(slicer).Slice(start, int(r.idx))
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
	var i int
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
	copy(p, buf)
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

type continuations map[string]*continuation

func (c continuations) put(h Header, b []byte) (Record, bool) {
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
