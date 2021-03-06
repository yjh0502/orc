package orc

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
	"io/ioutil"

	"github.com/golang/snappy"
)

// CompressionCodec is an interface that provides methods for creating
// an Encoder or Decoder of the CompressionCodec implementation.
type CompressionCodec interface {
	Encoder(w io.Writer) io.Writer
	Decoder(r io.Reader) io.Reader
}

// CompressionNone is a CompressionCodec that implements no compression.
type CompressionNone struct{}

// Encoder implements the CompressionCodec interface.
func (c CompressionNone) Encoder(w io.Writer) io.Writer {
	return w
}

// Decoder implements the CompressionCodec interface.
func (c CompressionNone) Decoder(r io.Reader) io.Reader {
	return r
}

type CompressionZlib struct {
	level    int
	strategy int
}

// Encoder implements the CompressionCodec interface. This is currently not implemented.
func (c CompressionZlib) Encoder(w io.Writer) io.Writer {
	return w
}

// Decoder implements the CompressionCodec interface.
func (c CompressionZlib) Decoder(r io.Reader) io.Reader {
	return &CompressionZlibDecoder{source: r}
}

// CompressionSnappy implements the CompressionCodec for Zlib compression.
type CompressionZlibDecoder struct {
	source      io.Reader
	decoded     io.Reader
	isOriginal  bool
	chunkLength int
	remaining   int64
}

func (c *CompressionZlibDecoder) readHeader() (int, error) {
	header := make([]byte, 4, 4)
	_, err := c.source.Read(header[:3])
	if err != nil {
		return 0, err
	}
	headerVal := binary.LittleEndian.Uint32(header)
	c.isOriginal = headerVal%2 == 1
	c.chunkLength = int(headerVal / 2)
	if !c.isOriginal {
		c.decoded = flate.NewReader(io.LimitReader(c.source, int64(c.chunkLength)))
	} else {
		c.decoded = io.LimitReader(c.source, int64(c.chunkLength))
	}
	return 0, nil
}

func (c *CompressionZlibDecoder) Read(p []byte) (int, error) {
	if c.decoded == nil {
		return c.readHeader()
	}
	n, err := c.decoded.Read(p)
	if err == io.EOF {
		c.decoded = nil
		return n, nil
	}
	return n, err
}

// CompressionSnappy implements the CompressionCodec for Snappy compression.
type CompressionSnappy struct{}

// Encoder implements the CompressionCodec interface. This is currently not implemented.
func (c CompressionSnappy) Encoder(w io.Writer) io.Writer {
	return w
}

// Decoder implements the CompressionCodec interface.
func (c CompressionSnappy) Decoder(r io.Reader) io.Reader {
	return &CompressionSnappyDecoder{source: r}
}

// CompressionSnappyDecoder implements the decoder for CompressionSnappy.
type CompressionSnappyDecoder struct {
	source      io.Reader
	decoded     io.Reader
	isOriginal  bool
	chunkLength int
	remaining   int64
}

func (c *CompressionSnappyDecoder) readHeader() (int, error) {
	header := make([]byte, 4, 4)
	_, err := c.source.Read(header[:3])
	if err != nil {
		return 0, err
	}
	headerVal := binary.LittleEndian.Uint32(header)
	c.isOriginal = headerVal%2 == 1
	c.chunkLength = int(headerVal / 2)
	if !c.isOriginal {
		// ORC does not use snappy's framing as implemented in the
		// github.com/golang/snappy Reader implementation. As a result
		// we have to read and decompress the entire chunk.
		// TODO: find reader implementation with optional framing.
		r := io.LimitReader(c.source, int64(c.chunkLength))
		src, err := ioutil.ReadAll(r)
		if err != nil {
			return 0, err
		}
		decodedBytes, err := snappy.Decode(nil, src)
		if err != nil {
			return 0, err
		}
		c.decoded = bytes.NewReader(decodedBytes)
	} else {
		c.decoded = io.LimitReader(c.source, int64(c.chunkLength))
	}
	return 0, nil
}

func (c *CompressionSnappyDecoder) Read(p []byte) (int, error) {
	if c.decoded == nil {
		return c.readHeader()
	}
	n, err := c.decoded.Read(p)
	if err == io.EOF || err == snappy.ErrCorrupt {
		c.decoded = nil
		return n, nil
	}
	return n, err
}
