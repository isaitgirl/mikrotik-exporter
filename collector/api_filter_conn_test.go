package collector

import (
	"bufio"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	routeros "gopkg.in/routeros.v2"
	"gopkg.in/routeros.v2/proto"
)

func TestAPIFilterConnKeepsSyncRepliesAligned(t *testing.T) {
	// TCP loopback, not net.Pipe: the trailing !done is only read with the next command, like a real socket.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	c, err := routeros.NewClient(newAPIFilterConn(client))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s := &rawServer{bufio.NewReader(server), proto.NewWriter(server), server}

	go func() {
		defer s.Close()
		s.expect(t, "/empty/print")
		s.writeSentence(t, "!empty")
		s.writeSentence(t, "!done")
		s.expect(t, "/missing/print")
		s.writeSentence(t, "!trap", "=message=no such command prefix")
		s.writeSentence(t, "!done")
		s.expect(t, "/data/print")
		s.writeSentence(t, "!re", "=name="+string(make([]byte, 200)))
		s.writeSentence(t, "!re", "=name=b")
		s.writeSentence(t, "!done")
	}()

	reply, err := c.Run("/empty/print")
	assert.NoError(t, err)
	assert.Empty(t, reply.Re)

	_, err = c.Run("/missing/print")
	assert.ErrorContains(t, err, "no such command")

	reply, err = c.Run("/data/print")
	assert.NoError(t, err)
	if assert.Len(t, reply.Re, 2) {
		assert.Len(t, reply.Re[0].Map["name"], 200)
		assert.Equal(t, "b", reply.Re[1].Map["name"])
	}
}
