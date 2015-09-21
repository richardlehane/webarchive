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
	"errors"
	"io"
	"time"
)

var (
	ErrVersionBlock = errors.New("webarchive: invalid ARC version block")
	ErrARCHeader    = errors.New("webarchive: invalid ARC header")
	ErrNotSlicer    = errors.New("webarchive: underlying reader must be a slicer to expose Slice and EOFSlice methods")
	ErrWARCHeader   = errors.New("webarchive: invalid WARC header")
	ErrWARCRecord   = errors.New("webarchive: error parsing WARC record")
)

func NewReader(r io.Reader) (Reader, error) {
	w, err := NewWARCReader(r)
	if err != nil {
		return NewARCReader(r)
	}
	return w, err
}

type Reader interface {
	Reset(io.Reader) error
	Next() (Record, error)
	NextPayload() (Record, error) // skip non-resonse/resource records; merge continuations; strip non-body content from record
}

type Record interface {
	Header
	Content
}

type Header interface {
	URL() string
	Date() time.Time
	Size() int64
	Fields() map[string][]string
}

type Content interface {
	Read(p []byte) (n int, err error)
	Slice(off int64, l int) ([]byte, error)
	EofSlice(off int64, l int) ([]byte, error)
}
