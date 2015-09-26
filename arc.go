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

// ARCTime is a time format string for the ARC time format
const ARCTime = "20060102150405"

type ARCHeader interface {
	Header
	size() int64
	setfields([]byte)
}

type ARC struct {
	Path       string
	Address    string
	FileDate   time.Time // YYYYMMDDhhmmss
	Version    int
	OriginCode string
}

// Version 1 URL record
type URL1 struct {
	url    string
	IP     string    // dotted-quad (eg 192.216.46.98 or 0.0.0.0)
	date   time.Time //  YYYYMMDDhhmmss (Greenwich Mean Time)
	MIME   string    // "no-type"|MIME type of data (e.g., "text/html")
	sz     int64
	fields []byte
}

func (u *URL1) URL() string     { return u.url }
func (u *URL1) Date() time.Time { return u.date }
func (u *URL1) Fields() map[string][]string {
	var fields map[string][]string
	if len(u.fields) > 0 {
		fields = getAllValues(u.fields)
	} else {
		fields = make(map[string][]string)
	}
	fields["URL"] = []string{u.url}
	fields["IP"] = []string{u.IP}
	fields["Date"] = []string{u.date.Format(ARCTime)}
	fields["MIME"] = []string{u.MIME}
	fields["Size"] = []string{strconv.FormatInt(u.sz, 10)}
	return fields
}
func (u *URL1) size() int64        { return u.sz }
func (u *URL1) setfields(f []byte) { u.fields = f }

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
	fields["Offset"] = []string{strconv.FormatInt(u.Offset, 10)}
	fields["Filename"] = []string{u.Location}
	return fields
}

type ARCReader struct {
	*ARC
	*reader
	ARCHeader
}

func NewARCReader(r io.Reader) (*ARCReader, error) {
	rdr, err := newReader(r)
	if err != nil {
		return nil, err
	}
	return newARCReader(rdr)
}

func newARCReader(r *reader) (*ARCReader, error) {
	arc := &ARCReader{reader: r}
	var err error
	arc.ARC, err = arc.readVersionBlock()
	return arc, err
}

func (a *ARCReader) Reset(r io.Reader) error {
	a.reader.reset(r)
	return a.reset()
}

func (a *ARCReader) reset() error {
	var err error
	a.ARC, err = a.readVersionBlock()
	return err
}

func (a *ARCReader) Next() (Record, error) {
	buf, err := a.next()
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(bytes.TrimSpace(buf), []byte(" "))
	if a.Version == 1 {
		a.ARCHeader, err = makeUrl1(parts)
	} else {
		a.ARCHeader, err = makeUrl2(parts)
	}
	if err != nil {
		return nil, err
	}
	a.thisIdx, a.sz = 0, a.size()
	return a, err
}

func (a *ARCReader) NextPayload() (Record, error) {
	r, err := a.Next()
	if err != nil {
		return r, err
	}
	if v, err := a.peek(5); err == nil && string(v) == "HTTP/" {
		f, err := a.storeLines(0, true)
		if err != nil {
			return r, err
		}
		a.setfields(f)
	}
	return r, err
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
	t, err := time.Parse(ARCTime, string(line1[2]))
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

func makeUrl1(p [][]byte) (*URL1, error) {
	if len(p) < 5 {
		return nil, ErrARCHeader
	}
	date, err := time.Parse(ARCTime, string(p[2]))
	if err != nil {
		return nil, ErrARCHeader
	}
	l, err := strconv.ParseInt(string(p[len(p)-1]), 10, 64)
	if err != nil {
		return nil, ErrARCHeader
	}
	return &URL1{
		url:  string(p[0]),
		IP:   string(p[1]),
		date: date,
		MIME: string(p[3]),
		sz:   l,
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
