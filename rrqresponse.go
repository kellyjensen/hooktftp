
package main

import (
	"net"
	"fmt"
	"encoding/binary"
	"strconv"
)

type Connection interface {
	ReadFrom(b []byte) (n int, addr net.Addr, err error)
	Write(p []byte) (n int, err error)
}

type RRQresponse struct {
	conn      Connection
	buffer    []byte
	pos       int
	ack       []byte
	blocksize int
	blocknum  uint16
}

func (res *RRQresponse) Write(p []byte) (int, error) {
	out := res.buffer[4:]

	bytecount := res.pos + len(p)

	if bytecount < int(res.blocksize) {
		res.pos += copy(out[res.pos:], p)
		return len(p), nil
	}

	copied := copy(out[res.pos:], p)
	remaining := p[copied:]

	res.pos += copied

	if res.pos == int(res.blocksize) {

		_, err := res.writeBuffer()
		if err != nil {
			return 0, err
		}

		res.pos = 0
		if len(remaining) != 0 {
			return res.Write(remaining)
		}
	}

	return len(p), nil
}

func (res *RRQresponse) writeBuffer() (int, error) {
	res.blocknum++
	binary.BigEndian.PutUint16(res.buffer, DATA)
	binary.BigEndian.PutUint16(res.buffer[2:], res.blocknum)

	out := res.buffer[:res.pos+4]

	written, err := res.conn.Write(out)
	if err != nil {
		fmt.Println("failed to write to connection", err)
		return 0, err
	}


	_, _, err = res.conn.ReadFrom(res.ack)

	if err != nil {
		fmt.Println("failed to read ack", err)
		return 0, err
	}

	// TODO: assert ack

	return written, nil

}

func (res *RRQresponse) WriteError(code uint16, message string) error {

	// http://tools.ietf.org/html/rfc1350#page-8
	errorbuffer := make([]byte, 2+2+len(message)+1)

	binary.BigEndian.PutUint16(errorbuffer, ERROR)
	binary.BigEndian.PutUint16(errorbuffer[2:], code)

	copy(errorbuffer[4:], message)
	errorbuffer[len(errorbuffer)-1] = 0

	_, err := res.conn.Write(errorbuffer)
	if err != nil {
		fmt.Println("Failed to write ERROR:", err)
		return err
	}

	return nil
}

func (res *RRQresponse) WriteOACK() error {
	if res.blocksize == DEFAULT_BLOCKSIZE {
		return nil
	}

	oackbuffer := make([]byte, 2)
	binary.BigEndian.PutUint16(oackbuffer, OACK)

	oackbuffer = append(oackbuffer, []byte("blksize")...)
	oackbuffer = append(oackbuffer, 0)
	oackbuffer = append(oackbuffer, []byte(strconv.Itoa(res.blocksize))...)
	oackbuffer = append(oackbuffer, 0)

	_, err := res.conn.Write(oackbuffer)
	if err != nil {
		fmt.Println("Failed to write OACK:", err)
		return err
	}

	_, _, err = res.conn.ReadFrom(res.ack)
	if err != nil {
		fmt.Println("Failed to read ACK for OACK:", err)
		return err
	}

	// TODO: assert ack

	return nil
}

func (res *RRQresponse) End() (int, error) {
	// Signal end of the transmission. This can be neither empty block or
	// block smaller than res.blocksize
	return res.writeBuffer()
}

func NewRRQresponse(clientaddr *net.UDPAddr, blocksize int) (*RRQresponse, error) {

	listenaddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", listenaddr, clientaddr)

	if err != nil {
		fmt.Println("failed to create client conn", err)
		return nil, err
	}

	return &RRQresponse{
		conn,
		make([]byte, blocksize+4),
		0,
		make([]byte, 4),
		blocksize,
		0,
	}, nil

}

