package collector

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

// apiFilterConn hides two API replies that the routeros.v2 sync client mishandles: "!empty"
// (RouterOS 7.18+, empty result) and the "!done" after "!trap"; both would shift every later reply.
type apiFilterConn struct {
	net.Conn
	r         *bufio.Reader
	buf       []byte
	afterTrap bool
}

func newAPIFilterConn(conn net.Conn) net.Conn {
	return &apiFilterConn{Conn: conn, r: bufio.NewReader(conn)}
}

func (c *apiFilterConn) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		raw, word, err := c.readSentence()
		if err != nil {
			return 0, err
		}
		if word == "!empty" || (word == "!done" && c.afterTrap) {
			c.afterTrap = false
			continue
		}
		c.afterTrap = word == "!trap"
		c.buf = raw
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

// readSentence returns the raw bytes of one sentence and its first word.
func (c *apiFilterConn) readSentence() ([]byte, string, error) {
	var raw []byte
	first := ""
	for i := 0; ; i++ {
		header, length, err := c.readLength()
		if err != nil {
			return nil, "", err
		}
		raw = append(raw, header...)
		if length == 0 {
			return raw, first, nil
		}
		word := make([]byte, length)
		if _, err := io.ReadFull(c.r, word); err != nil {
			return nil, "", err
		}
		raw = append(raw, word...)
		if i == 0 {
			first = string(word)
		}
	}
}

func (c *apiFilterConn) readLength() ([]byte, int, error) {
	b0, err := c.r.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	var extra int
	var length int
	switch {
	case b0&0x80 == 0x00:
		length = int(b0)
	case b0&0xC0 == 0x80:
		extra, length = 1, int(b0&^0xC0)
	case b0&0xE0 == 0xC0:
		extra, length = 2, int(b0&^0xE0)
	case b0&0xF0 == 0xE0:
		extra, length = 3, int(b0&^0xF0)
	case b0 == 0xF0:
		extra = 4
	default:
		return nil, 0, fmt.Errorf("invalid API word length prefix 0x%02x", b0)
	}
	header := []byte{b0}
	for range extra {
		b, err := c.r.ReadByte()
		if err != nil {
			return nil, 0, err
		}
		header = append(header, b)
		length = length<<8 | int(b)
	}
	return header, length, nil
}
