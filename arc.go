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
	"errors"
	"io"
	"strconv"
	"time"
)

const timefmt = "20060102150405"

type ARC struct {
	Path       string
	Address    string
	FileDate   time.Time // YYYYMMDDhhmmss
	Version    int
	OriginCode string
}

// Version 1 URL record
type URL1 struct {
	url  string
	IP   string    // dotted-quad (eg 192.216.46.98 or 0.0.0.0)
	date time.Time //  YYYYMMDDhhmmss (Greenwich Mean Time)
	MIME string    // "no-type"|MIME type of data (e.g., "text/html")
	size int64
}

func (u *URL1) URL() string     { return u.url }
func (u *URL1) Date() time.Time { return u.date }
func (u *URL1) Size() int64     { return u.size }
func (u *URL1) Fields() map[string][]string {
	return map[string][]string{
		"URL":  []string{u.url},
		"IP":   []string{u.IP},
		"Date": []string{u.date.Format(timefmt)},
		"MIME": []string{u.MIME},
		"Size": []string{strconv.FormatInt(u.size, 64)},
	}
}

// Version 2 URL record
type URL2 struct {
	*URL1
	StatusCode int
	Checksum   string
	Location   string
	Offset     int64
	Filename   string
}

func (u *URL2) Fields() map[string][]string {
	fields := u.URL1.Fields()
	fields["StatusCode"] = []string{strconv.Itoa(u.StatusCode)}
	fields["Checksum"] = []string{u.Checksum}
	fields["Location"] = []string{u.Location}
	fields["Offset"] = []string{strconv.FormatInt(u.Offset, 64)}
	fields["Filename"] = []string{u.Location}
	return fields
}

type ARCReader struct {
	*ARC
	*reader
	Header
}

func NewARCReader(r io.Reader) (*ARCReader, error) {
	arc := &ARCReader{reader: newReader(r)}
	var err error
	arc.ARC, err = arc.readVersionBlock()
	return arc, err
}

func (a *ARCReader) Reset(r io.Reader) error {
	a.reader.reset(r)
	var err error
	a.ARC, err = a.readVersionBlock()
	return err
}

func (r *ARCReader) Next() (Record, error) {
	// advance if haven't read the previous record
	if r.thisIdx < r.sz {
		if r.slicer {
			r.idx += r.sz - r.thisIdx
		} else {
			discard(r.buf, int(r.sz-r.thisIdx))
		}
	}
	u, err := r.readURL()
	if err != nil {
		return nil, err
	}
	r.thisIdx, r.sz = 0, u.Size()
	return r, err
}

func (a *ARCReader) NextPayload() (Record, error) {
	return a.Next()
}

func (r *ARCReader) readVersionBlock() (*ARC, error) {
	buf, _ := r.readLine()
	if len(buf) == 0 {
		return nil, ErrVersionBlock
	}
	line1 := bytes.Split(buf, []byte(" "))
	if len(line1) < 3 {
		return nil, ErrVersionBlock
	}
	t, err := time.Parse(timefmt, string(line1[2]))
	if err != nil {
		return nil, ErrVersionBlock
	}
	buf, _ = r.readLine()
	line2 := bytes.Split(buf, []byte(" "))
	if len(line2) < 3 {
		return nil, ErrVersionBlock
	}
	version, err := strconv.Atoi(string(line2[0]))
	if err != nil {
		return nil, ErrVersionBlock
	}
	l, err := strconv.Atoi(string(bytes.TrimSpace(line1[len(line1)-1])))
	if err != nil {
		return nil, ErrVersionBlock
	}
	// now scan ahead to first doc
	l -= len(buf)
	if r.slicer {
		r.idx += int64(l)
	} else {
		discard(r.buf, l)
	}
	return &ARC{
		Path:       string(line1[0]),
		Address:    string(line1[1]),
		FileDate:   t,
		Version:    version,
		OriginCode: string(bytes.TrimSpace(line2[len(line2)-1])),
	}, nil
}

func (r *ARCReader) readURL() (Header, error) {
	var buf []byte
	var err error
	for buf, err = r.readLine(); err == nil && len(bytes.TrimSpace(buf)) == 0; buf, err = r.readLine() {
	}
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(bytes.TrimSpace(buf), []byte(" "))
	if r.Version == 1 {
		return makeUrl1(parts)
	}
	return makeUrl2(parts)
}

func makeUrl1(p [][]byte) (*URL1, error) {
	if len(p) < 5 {
		return nil, errors.New(string(p[0]) + string(p[1]))
	}
	date, err := time.Parse(timefmt, string(p[2]))
	if err != nil {
		return nil, ErrARCHeader
	}
	l, err := strconv.ParseInt(string(p[len(p)-1]), 10, 64)
	if err != nil {
		return nil, ErrARCHeader
	}
	return &URL1{
		url:  string(p[2]),
		IP:   string(p[1]),
		date: date,
		MIME: string(p[3]),
		size: l,
	}, nil
}

func makeUrl2(p [][]byte) (*URL2, error) {
	if len(p) != 10 {
		return nil, ErrARCHeader
	}
	u1, err := makeUrl1(p)
	if err != nil {
		return nil, ErrARCHeader
	}
	status, err := strconv.Atoi(string(p[4]))
	if err != nil {
		return nil, ErrARCHeader
	}
	offset, err := strconv.ParseInt(string(p[7]), 10, 64)
	if err != nil {
		return nil, ErrARCHeader
	}
	return &URL2{
		URL1:       u1,
		StatusCode: status,
		Checksum:   string(p[5]),
		Location:   string(p[6]),
		Offset:     offset,
		Filename:   string(p[8]),
	}, nil
}
