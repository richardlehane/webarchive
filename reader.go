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

/*


// ReadLine reads a single line from r,
// eliding the final \n or \r\n from the returned string.
func (r *Reader) ReadLine() (string, error) {
	line, err := r.readLineSlice()
	return string(line), err
}

// ReadLineBytes is like ReadLine but returns a []byte instead of a string.
func (r *Reader) ReadLineBytes() ([]byte, error) {
	line, err := r.readLineSlice()
	if line != nil {
		buf := make([]byte, len(line))
		copy(buf, line)
		line = buf
	}
	return line, err
}

func (r *Reader) readLineSlice() ([]byte, error) {
	r.closeDot()
	var line []byte
	for {
		l, more, err := r.R.ReadLine()
		if err != nil {
			return nil, err
		}
		// Avoid the copy if the first call produced a full line.
		if line == nil && !more {
			return l, nil
		}
		line = append(line, l...)
		if !more {
			break
		}
	}
	return line, nil
}

func (r *Reader) ReadContinuedLine() (string, error) {
	line, err := r.readContinuedLineSlice()
	return string(line), err
}

// trim returns s with leading and trailing spaces and tabs removed.
// It does not assume Unicode or UTF-8.
func trim(s []byte) []byte {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	n := len(s)
	for n > i && (s[n-1] == ' ' || s[n-1] == '\t') {
		n--
	}
	return s[i:n]
}



// ReadMIMEHeader reads a MIME-style header from r.
// The header is a sequence of possibly continued Key: Value lines
// ending in a blank line.
// The returned map m maps CanonicalMIMEHeaderKey(key) to a
// sequence of values in the same order encountered in the input.
//
// For example, consider this input:
//
//	My-Key: Value 1
//	Long-Key: Even
//	       Longer Value
//	My-Key: Value 2
//
// Given that input, ReadMIMEHeader returns the map:
//
//	map[string][]string{
//		"My-Key": {"Value 1", "Value 2"},
//		"Long-Key": {"Even Longer Value"},
//	}
//
func (r *Reader) ReadMIMEHeader() (MIMEHeader, error) {
	// Avoid lots of small slice allocations later by allocating one
	// large one ahead of time which we'll cut up into smaller
	// slices. If this isn't big enough later, we allocate small ones.
	var strs []string
	hint := r.upcomingHeaderNewlines()
	if hint > 0 {
		strs = make([]string, hint)
	}

	m := make(MIMEHeader, hint)
	for {
		kv, err := r.readContinuedLineSlice()
		if len(kv) == 0 {
			return m, err
		}

		// Key ends at first colon; should not have spaces but
		// they appear in the wild, violating specs, so we
		// remove them if present.
		i := bytes.IndexByte(kv, ':')
		if i < 0 {
			return m, ProtocolError("malformed MIME header line: " + string(kv))
		}
		endKey := i
		for endKey > 0 && kv[endKey-1] == ' ' {
			endKey--
		}
		key := canonicalMIMEHeaderKey(kv[:endKey])

		// As per RFC 7230 field-name is a token, tokens consist of one or more chars.
		// We could return a ProtocolError here, but better to be liberal in what we
		// accept, so if we get an empty key, skip it.
		if key == "" {
			continue
		}

		// Skip initial spaces in value.
		i++ // skip colon
		for i < len(kv) && (kv[i] == ' ' || kv[i] == '\t') {
			i++
		}
		value := string(kv[i:])

		vv := m[key]
		if vv == nil && len(strs) > 0 {
			// More than likely this will be a single-element key.
			// Most headers aren't multi-valued.
			// Set the capacity on strs[0] to 1, so any future append
			// won't extend the slice into the other strings.
			vv, strs = strs[:1:1], strs[1:]
			vv[0] = value
			m[key] = vv
		} else {
			m[key] = append(vv, value)
		}

		if err != nil {
			return m, err
		}
	}
}



// CanonicalMIMEHeaderKey returns the canonical format of the
// MIME header key s.  The canonicalization converts the first
// letter and any letter following a hyphen to upper case;
// the rest are converted to lowercase.  For example, the
// canonical key for "accept-encoding" is "Accept-Encoding".
// MIME header keys are assumed to be ASCII only.
// If s contains a space or invalid header field bytes, it is
// returned without modifications.
func CanonicalMIMEHeaderKey(s string) string {
	// Quick check for canonical encoding.
	upper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !validHeaderFieldByte(c) {
			return s
		}
		if upper && 'a' <= c && c <= 'z' {
			return canonicalMIMEHeaderKey([]byte(s))
		}
		if !upper && 'A' <= c && c <= 'Z' {
			return canonicalMIMEHeaderKey([]byte(s))
		}
		upper = c == '-'
	}
	return s
}

const toLower = 'a' - 'A'

// validHeaderFieldByte reports whether b is a valid byte in a header
// field key. This is actually stricter than RFC 7230, which says:
//   tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
//           "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
//   token = 1*tchar
// TODO: revisit in Go 1.6+ and possibly expand this. But note that many
// servers have historically dropped '_' to prevent ambiguities when mapping
// to CGI environment variables.
func validHeaderFieldByte(b byte) bool {
	return ('A' <= b && b <= 'Z') ||
		('a' <= b && b <= 'z') ||
		('0' <= b && b <= '9') ||
		b == '-'
}

// canonicalMIMEHeaderKey is like CanonicalMIMEHeaderKey but is
// allowed to mutate the provided byte slice before returning the
// string.
//
// For invalid inputs (if a contains spaces or non-token bytes), a
// is unchanged and a string copy is returned.
func canonicalMIMEHeaderKey(a []byte) string {
	// See if a looks like a header key. If not, return it unchanged.
	for _, c := range a {
		if validHeaderFieldByte(c) {
			continue
		}
		// Don't canonicalize.
		return string(a)
	}

	upper := true
	for i, c := range a {
		// Canonicalize: first letter upper case
		// and upper case after each dash.
		// (Host, User-Agent, If-Modified-Since).
		// MIME headers are ASCII only, so no Unicode issues.
		if upper && 'a' <= c && c <= 'z' {
			c -= toLower
		} else if !upper && 'A' <= c && c <= 'Z' {
			c += toLower
		}
		a[i] = c
		upper = c == '-' // for next time
	}
	// The compiler recognizes m[string(byteSlice)] as a special
	// case, so a copy of a's bytes into a new string does not
	// happen in this map lookup:
	if v := commonHeader[string(a)]; v != "" {
		return v
	}
	return string(a)
}

// ReadResponse reads and returns an HTTP response from r.
// The req parameter optionally specifies the Request that corresponds
// to this Response. If nil, a GET request is assumed.
// Clients must call resp.Body.Close when finished reading resp.Body.
// After that call, clients can inspect resp.Trailer to find key/value
// pairs included in the response trailer.
func ReadResponse(r *bufio.Reader, req *Request) (*Response, error) {
	tp := textproto.NewReader(r)
	resp := &Response{
		Request: req,
	}

	// Parse the first line of the response.
	line, err := tp.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	f := strings.SplitN(line, " ", 3)
	if len(f) < 2 {
		return nil, &badStringError{"malformed HTTP response", line}
	}
	reasonPhrase := ""
	if len(f) > 2 {
		reasonPhrase = f[2]
	}
	resp.Status = f[1] + " " + reasonPhrase
	resp.StatusCode, err = strconv.Atoi(f[1])
	if err != nil {
		return nil, &badStringError{"malformed HTTP status code", f[1]}
	}

	resp.Proto = f[0]
	var ok bool
	if resp.ProtoMajor, resp.ProtoMinor, ok = ParseHTTPVersion(resp.Proto); !ok {
		return nil, &badStringError{"malformed HTTP version", resp.Proto}
	}

	// Parse the response headers.
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	resp.Header = Header(mimeHeader)

	fixPragmaCacheControl(resp.Header)

	err = readTransfer(resp, r)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
*/
