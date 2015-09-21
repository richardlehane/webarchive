// +build !go1.5

package webarchive

import (
	"bufio"
	"log"
)

var discardBuf []byte

func discard(r *bufio.Reader, i int) (int, error) {
	if len(discardBuf) < i {
		discardBuf = make([]byte, i)
	}
	l, err := fullRead(r, discardBuf[:i])
	if l != i {
		log.Fatalf("expecting to have discarded %d, discarded %d, %v", i, l, err)
	}
	return l, err
}
