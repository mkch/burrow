package fields

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"testing"
)

func TestDecoderRead(t *testing.T) {
	t.Parallel()

	var r io.Reader
	var decoder *Decoder
	var b uint32
	var err error

	// Test wrong arguments.
	decoder = NewDecoder(rand.Reader)
	b, err = decoder.ReadBits(0)
	if b != 0 || err == nil || decoder.IsClean() != true {
		t.Fatalf("Read 0 bits failed. Got: %v %v %v. Want 0, <error> true", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(100)
	if b != 0 || err == nil {
		t.Fatalf("Read 100 bits failed. Got: %v %v %v. Want 0, <error> true", b, err, decoder.IsClean())
	}
	// Test 32 bits.
	r = bytes.NewBuffer([]byte{1, 0xFE, 0xFF, 0xFF, 0xFF})
	decoder = NewDecoder(r)
	b, err = decoder.ReadBits(4)
	if b != 0x0 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Raad 4 bits before 64 failed. Got: 0x%x, %v %v. Want: 0x0, <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(32)
	if b != 0x1FEFFFFF || err != nil || decoder.IsClean() != false {
		t.Fatalf("Raad 64 bits before 64 failed. Got: 0x%x %v %v. Want: 0x1FEFFFFF, <nil> false", b, err, decoder.IsClean())
	}
	// 1,0,10, 0101, 111,1 1111 000,0 0000 0101 1010
	r = bytes.NewBuffer([]byte{0xA5, 0xFF, 0x00, 0x5A})
	decoder = NewDecoder(r)
	b, err = decoder.ReadBits(1)
	if b != 1 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Read bit #0 error: Got: 0x%x %v %v. Want: 0x1 <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(1)
	if b != 0 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Read bit #1 error: Got: 0x%x %v %v. Want: 0x0 <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(2)
	if b != 0x2 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Read bit #2-3 error: Got: 0x%x %v %v. Want: 0x2. <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(4)
	if b != 0x5 || err != nil || decoder.IsClean() != true {
		t.Fatalf("Read bit #4-7 error: Got: 0x%x %v %v. Want: 0x51, <nil> true", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(3)
	if b != 0x7 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Read bit #8-10 error: Got: 0x%x %v %v. Want: 0x7. <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(8)
	if b != 0xf8 || err != nil || decoder.IsClean() != false {
		t.Fatalf("Read bit #11-18 error: Got: 0x%x %v %v. Want: 0xf8 <nil> false", b, err, decoder.IsClean())
	}
	b, err = decoder.ReadBits(13)
	if b != 0x5A || err != nil || decoder.IsClean() != true {
		t.Fatalf("Read bit #19- error: Got: 0x%x %v %v. Want: 0x5A <nil> true", b, err, decoder.IsClean())
	}
}

func TestDecoderWrite(t *testing.T) {
	t.Parallel()

	var w *bytes.Buffer
	var encoder *Encoder
	var err error

	w = &bytes.Buffer{}
	encoder = NewEncoder(w)
	// 1
	err = encoder.WriteBits(1, 0x1)
	if err != nil || encoder.IsClean() != false {
		t.Fatalf("Write 1 bit failed. Got: %v %v Want: <nil> false", err, encoder.IsClean())
	}
	// 1000 1000 -> 1100 0100 0
	// 1100 0100 = 0xC4
	err = encoder.WriteBits(8, 0x88)
	if err != nil || !bytes.Equal(w.Bytes(), []byte{0xC4}) || encoder.IsClean() != false {
		t.Fatalf("Write 8 bits failed. Got: %v %v %v. Want: [196] <nil> flase", w.Bytes(), err, encoder.IsClean())
	}
	// 0100 001 -> 1100 0100 0010 0001
	// 1100 0100 0010 0001 = 0xC421
	err = encoder.WriteBits(7, 0x21)
	if err != nil || !bytes.Equal(w.Bytes(), []byte{0xC4, 0x21}) || encoder.IsClean() != true {
		t.Fatalf("Write 7 bits failed. Got: %v %v %v. Want: [196 33] <nil> true", w.Bytes(), err, encoder.IsClean())
	}
}

func TestDecoderEncoder(t *testing.T) {
	t.Parallel()

	var rw *bytes.Buffer
	var decoder *Decoder
	var encoder *Encoder
	var err error

	rw = &bytes.Buffer{}
	decoder = NewDecoder(rw)
	encoder = NewEncoder(rw)

	err = encoder.WriteBits(1, 0x1)
	if err != nil {
		t.Fatalf("Encoder write flag failed: %v", err)
	}
	err = encoder.WriteBits(31, 0xCC)
	if err != nil {
		t.Fatalf("Encoder write data failed: %v", err)
	}

	var n uint32
	n, err = decoder.ReadBits(1)
	if n != 1 || err != nil {
		t.Fatalf("Decoder read flag failed: %v %v", n, err)
	}
	n, err = decoder.ReadBits(31)
	if n != 0xCC || err != nil {
		t.Fatalf("Decoder read data failed: %v %v", n, err)
	}

}

type structB struct {
	Flags byte   `field:"bits:8"`
	Data  int    `field:"bits:16"`
	Str   string `field:"lenbits:16"`
}

type structX struct {
	a int
}

type structA struct {
	structX    `field:"-"`
	C          byte       `field:"bits:1"`
	T          byte       `field:"bits:7"`
	D          []structB  `field:"lenbits:8"`
	Dptr       []*structB `field:"lenbits:16"`
	Limit      uint       `field:"bits:8,limit"`
	AfterLimit int        `field:"bits:32"`
}

func TestDecoderDecode(t *testing.T) {
	t.Parallel()

	r := bytes.NewBuffer(
		[]byte{0xA4, // C & T
			0x2,                                    // len(D)
			0x3, 0x1, 0x2, 0x0, 0x3, 'a', 'b', 'c', // D[0]
			0x2, 0x03, 0x4, 0x0, 0x0, // D[1]
			0x0, 0x2, // len(Dptr)
			0x3, 0x1, 0x2, 0x0, 0x3, 'a', 'b', 'c', // D[0]
			0x2, 0x03, 0x4, 0x0, 0x0, // D[1]
			// Limit
			0x4, // Limit 4 bytes(32 bits)
			0x10, 0x20, 0x30, 0xFF,
			// Extra
			0x00, 0xFF})
	decoder := NewDecoder(r)

	var a structA
	err := decoder.Decode(&a)
	if err != nil ||
		a.C != 1 || a.T != 0x24 ||
		len(a.D) != 2 ||
		a.D[0].Flags != 0x3 || a.D[0].Data != 258 || a.D[0].Str != "abc" ||
		a.D[1].Flags != 0x2 || a.D[1].Data != 772 || a.D[1].Str != "" ||
		a.Limit != 4 || a.AfterLimit != 0x102030FF ||
		len(a.Dptr) != 2 ||
		a.Dptr[0].Flags != 0x3 || a.Dptr[0].Data != 258 || a.Dptr[0].Str != "abc" ||
		a.Dptr[1].Flags != 0x2 || a.Dptr[1].Data != 772 || a.Dptr[1].Str != "" ||
		a.Limit != 4 || a.AfterLimit != 0x102030FF {
		t.Fatalf("Decode structA failed. Got: %#v %v", a, err)
	}
	if !decoder.IsClean() {
		t.Fatal("Decoder is not clean aftr reading struct\n")
	}
}

func TestEncoderEncode(t *testing.T) {
	t.Parallel()

	var a structA = structA{
		C: 0,
		T: 0x64,
		D: []structB{
			structB{
				Flags: 0xFF,
				Data:  0xFF10,
				Str:   "123456",
			},
		},
		Dptr: []*structB{
			&structB{
				Flags: 0xF1,
				Data:  0xABCD,
				Str:   "abcd",
			},
		},
		Limit:      4,
		AfterLimit: 0xC1AB00,
	}
	var rw = &bytes.Buffer{}
	var encoder = NewEncoder(rw)
	var decoder = NewDecoder(rw)
	var err error

	if err = encoder.Encode(a); err != nil {
		t.Fatalf("Encoding a failed. %v\n", err)
	}
	if !encoder.IsClean() {
		t.Fatal("Encoder is not clean aftr writing struct")
	}

	var b structA
	if err = decoder.Decode(&b); err != nil {
		t.Fatalf("Decoding to b failed. %v\n", err)
	}

	if a.C != b.C || a.T != b.T || a.Limit != b.Limit || a.AfterLimit != b.AfterLimit ||
		len(a.D) != len(b.D) || a.D[0] != b.D[0] ||
		len(a.Dptr) != len(b.Dptr) || (*a.Dptr[0]) != (*b.Dptr[0]) {
		t.Fatalf("Decoded data is not equal to the data encoded . a=%#v b=%#v\n", a, b)
	}
}

type structWithZlib struct {
	Flags byte       `field:"bits:1"`
	Type  byte       `field:"bits:7"`
	L     uint16     `field:"bits:16,limit"`
	X     byte       `field:"bits:8"`
	B1    []structB  `field:"lenbits:8"`
	B2    []*structB `field:"lenbits:16,zlib"`
}

func TestZlib(t *testing.T) {
	t.Parallel()

	var a structWithZlib = structWithZlib{
		Flags: 0,
		Type:  29,
		B2: []*structB{
			&structB{
				Flags: 0,
				Str:   "aabbaabbaabb",
			},
		},
	}

	rw := &bytes.Buffer{}
	decoder := NewDecoder(rw)
	encoder := NewEncoder(rw)

	var err error
	if err = encoder.Encode(&a); err != nil {
		t.Fatalf("Encoding zlib data failed: %v\n", err)
	}

	var b structWithZlib
	if err = decoder.Decode(&b); err != nil {
		t.Fatalf("Decoding zlib data failed: %v\n", err)
	}

	if a.B2[0].Flags != b.B2[0].Flags || len(a.B1) != 0 {
		t.Fatalf("Encoded zlib data does not equal to decoded: a=%v b=%v\n", a, b)
	}
}

type EmptyReader struct{}

func (r EmptyReader) Read(data []byte) (int, error) {
	return len(data), nil
}

var benchmarkDecoder = NewDecoder(EmptyReader{})

func BenchmarkDecoder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkDecoder.ReadBits(31)
	}
}

var benchmarkEncoder = NewEncoder(ioutil.Discard)

func BenchmarkEncoder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkEncoder.WriteBits(31, 0xFF)
	}
}
