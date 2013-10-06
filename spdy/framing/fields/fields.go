package fields

import (
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
)

type switchReader struct {
	io.Reader
}

func (r *switchReader) Switch(reader io.Reader) {
	r.Reader = reader
}

type Decoder struct {
	bo       binary.ByteOrder
	b        byte // Contains left over of a previous not-byte-aligned reading.
	leftOver int  // The count of left over bits in b.
	readBuf  [4]byte
	r        io.Reader
	sr       switchReader
	z        io.ReadCloser
	zDict    []byte
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{bo: binary.BigEndian, r: r}
}

func (d *Decoder) ReadBits(count int) (n uint32, err error) {
	if count <= 0 || count > 32 {
		return 0, errors.New("Invalid bit count to read!")
	}
	bitsNeeded := count - d.leftOver
	// Left over is enough
	if bitsNeeded <= 0 {
		n = uint32((d.b & (0xFF >> uint(8-d.leftOver))) >> uint(-bitsNeeded))
		d.leftOver = -bitsNeeded
		return
	}

	bytesNeeded := bitsNeeded / 8
	if bitsNeeded%8 != 0 {
		bytesNeeded++
	}

	buf := d.readBuf[len(d.readBuf)-bytesNeeded:]
	// Read
	if _, err = io.ReadFull(d.r, buf); err != nil {
		return 0, err
	}

	// Convert read byte to integer.
	n = d.bo.Uint32(d.readBuf[:])
	// Clear the garbage out of this read.
	// This is faster than ``for i, _ := range d.readBuf { d.readBuf[i] = 0 }'' beofre read.
	n &= (0xFFFFFFFF >> uint((len(d.readBuf)-bytesNeeded)*8))
	// Shift out the extra bits(, which will be the left of next read).
	leftOver := bytesNeeded*8 - bitsNeeded
	if leftOver > 0 {
		n >>= uint(leftOver)
	}
	// Patch the left over of previous read.
	leftOverPatch := uint32(d.b&(0xFF>>uint(8-d.leftOver))) << uint(count-d.leftOver)
	n |= leftOverPatch

	d.leftOver = leftOver
	if d.leftOver > 0 {
		d.b = d.readBuf[len(d.readBuf)-1]
	}
	return
}

func (d *Decoder) Read(data []byte) (int, error) {
	if !d.IsClean() {
		return 0, errors.New("Decoder is not clean")
	}
	return d.r.Read(data)
}

func (d *Decoder) SetZlibDict(dict []byte) {
	d.zDict = dict
}

func (d *Decoder) IsClean() bool {
	return d.leftOver == 0
}

func (d *Decoder) zlibReader(reader io.Reader) (zreader io.Reader, err error) {
	if d.z == nil {
		d.sr.Switch(reader)
		if d.z, err = zlib.NewReaderDict(&d.sr, d.zDict); err != nil {
			return
		}
	} else {
		d.sr.Switch(reader)
	}
	return d.z, nil
}

type Encoder struct {
	bo       binary.ByteOrder
	b        byte
	pending  int
	writeBuf [4]byte
	w        io.Writer
	z        *zlib.Writer
	sw       switchWriter
	zDict    []byte
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{bo: binary.BigEndian, w: w}
}

func (e *Encoder) IsClean() bool {
	return e.pending == 0
}

func (e *Encoder) WriteBits(count int, n uint32) (err error) {
	if count <= 0 || count > 32 {
		return errors.New("Invalid bit count to write!")
	}

	bitsToWrite := count + e.pending
	if bitsToWrite < 8 {
		// Not enough bits to write.
		e.b |= (byte(n) << uint(8-bitsToWrite))
		e.pending = bitsToWrite
		return
	}

	buf := e.writeBuf[:]
	var bytesToWrite = bitsToWrite / 8
	var pending = bitsToWrite % 8

	if pending < 0 {
		pending = 0
	}
	b := byte(n << uint(32-pending) >> uint(24-pending))
	n = n>>uint(pending) | uint32(e.b)<<uint(count-pending-8+e.pending)
	n <<= uint(32 - bytesToWrite*8)
	e.bo.PutUint32(buf, n)
	if bytesToWrite > 4 {
		bytesToWrite = 4
	}
	buf = buf[:bytesToWrite]
	if _, err = e.w.Write(buf); err != nil {
		return
	}
	e.b = b
	e.pending = pending
	return
}

type switchWriter struct {
	io.Writer
}

func (w *switchWriter) Switch(writer io.Writer) {
	w.Writer = writer
}

func (e *Encoder) Write(data []byte) (int, error) {
	if !e.IsClean() {
		return 0, errors.New("Encoder is not clean")
	}
	return e.w.Write(data)
}

func (e *Encoder) SetZlibDict(dict []byte) {
	e.zDict = dict
}

func (e *Encoder) zlibWriter(w io.Writer) (z *zlib.Writer, err error) {
	if e.z == nil {
		e.sw.Switch(w)
		if e.z, err = zlib.NewWriterLevelDict(&e.sw, 7, e.zDict); err != nil {
			return
		}
	} else {
		e.sw.Switch(w)
	}
	return e.z, nil
}
