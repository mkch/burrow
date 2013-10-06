/*
Package fields implements wire format decoding and encoding.

This package does bitwise wire format decoding and encoding with reflection
and struct tag of golang.

Encoder.Encode() and Decoder.Decode() recognize "field" struct tags. the "field"
struct tags are in the form of `field:"spec:value,spec,spec"`. All structs used
must be byte-aligned.

Specs currently are:
	bits
"bits" spec can only be used on integer fields, specifying the length of this
field, in bits. This spec must come with a integer value.
	limit
"limit" spec can only be used on unsigned integer fields. There can only be at
most one field in a struct tagged by "limit". The value of the field tagged by
"limit" is the length of content in the struct after this field, in ``bytes''.
This spec must come with no value.
	lenbits
"lenbits" spec can only be used on slice and string fields, specifying the bit
length of the length. Slices and strings are encoded as "length followed by content".
This spec must come with a integer value.
	zlib
"zlib" spec specifying this field is compressed by zlib. It can only be used on
the last slice field of a struct, which must have a "limit" field before "zlib"
field. Empty "zlib" slice field is omitted completely. This spec must come with
no value.
	-
"-" spec marks a field as omitted explictly.

Data types supported are:
	1. All integer types(int, int16, uint, etc.)
	2. string
	3. slice of 4
	4. struct contains unomitted fileds of type 1 2 3 4
*/
package fields
