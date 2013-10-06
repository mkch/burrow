package fields

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"reflect"
)

type triggerWriter struct {
	io.Writer
	Written bool
}

func (w *triggerWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if n > 0 {
		w.Written = true
	}
	return
}

type SpecError struct {
	msg string
}

func (e SpecError) Error() string {
	return e.msg
}

func newSpecError(msg string) SpecError {
	return SpecError{msg}
}

func specErrorf(format string, a ...interface{}) SpecError {
	return SpecError{fmt.Sprintf(format, a...)}
}

func (d *Decoder) Decode(v interface{}) (err error) {
	t := reflect.TypeOf(v)
	value := reflect.ValueOf(v)
	if t.Kind() == reflect.Ptr {
		if value.IsNil() {
			return errors.New("Nil pointer")
		}
		t = t.Elem()
		value = reflect.Indirect(value)
	}
	if t.Kind() != reflect.Struct {
		return specErrorf("Unsupported type %v", reflect.TypeOf(v))
	}
	err = d.decodeStruct(value, nil)
	if !d.IsClean() {
		panic(specErrorf("Struct %v is not byte-aligned", t))
	}
	return
}

func (e *Encoder) Encode(v interface{}) (err error) {
	t := reflect.TypeOf(v)
	value := reflect.ValueOf(v)
	if t.Kind() == reflect.Ptr {
		if value.IsNil() {
			return errors.New("Nil pointer")
		}
		t = t.Elem()
		value = reflect.Indirect(value)
	}
	if t.Kind() != reflect.Struct {
		return specErrorf("Unsupported type %v", reflect.TypeOf(v))
	}
	err = e.encodeStruct(value, nil)
	if !e.IsClean() {
		panic(specErrorf("Struct %v is not byte-aligned", t))
	}
	return
}

func (d *Decoder) decodeStruct(v reflect.Value, _unused *fieldInfo) (err error) {
	t := v.Type()
	var si structInfo
	if si, err = parseStruct(t); err != nil {
		return
	}

	var rBackup = d.r
	defer func() { d.r = rBackup }()

	var limited bool
	for i, fieldInfo := range si {
		if fieldInfo == nil {
			continue
		}
		var fv = v.Field(i)
		if fieldInfo.ptr {
			if fv.IsNil() {
				fv = reflect.New(fieldInfo.indirectType)
				v.Field(i).Set(fv)
			}
			fv = reflect.Indirect(fv)
		}
		// zlib
		var rBeforeZ io.Reader
		if fieldInfo.zlib {
			rBeforeZ = d.r
			if d.r, err = d.zlibReader(d.r); err != nil {
				return
			}
		}
		if err = fieldInfo.decode(d, fv, fieldInfo); err != nil {
			if !(limited && fieldInfo.zlib && err == errDecodeEOFBeforeArraySlice) {
				return
			}
			err = nil
		}
		if fieldInfo.zlib {
			d.r = rBeforeZ
		}
		// limit
		if fieldInfo.limit {
			var limit = fv.Uint()
			d.r = io.LimitReader(d.r, int64(limit))
			limited = true
		}
	}
	return
}

func (e *Encoder) encodeStruct(v reflect.Value, _unused *fieldInfo) (err error) {
	t := v.Type()
	var si structInfo
	if si, err = parseStruct(t); err != nil {
		return
	}

	var wBeforeLimit = e.w

	var limitBits int
	for i, fieldInfo := range si {
		if fieldInfo == nil {
			continue
		}

		var fv = v.Field(i)
		if fieldInfo.ptr {
			if fv.IsNil() {
				fv = reflect.New(fieldInfo.indirectType)
				v.Field(i).Set(fv)
			}
			fv = reflect.Indirect(fv)
		}

		// Limit
		if fieldInfo.limit {
			limitBits = fieldInfo.bits
			e.w = &bytes.Buffer{}
			continue
		}
		var w io.Writer
		var z *zlib.Writer
		// zlib
		if fieldInfo.zlib {
			w = e.w
			if z, err = e.zlibWriter(e.w); err != nil {
				return
			}
			e.w = z
		}

		var zEmpty bool
		if err = fieldInfo.encode(e, fv, fieldInfo); err != nil {
			if err == errEncodeEmptySliceArrayOmitted {
				zEmpty = true
			} else {
				return
			}
		}

		if fieldInfo.zlib {
			if !zEmpty {
				z.Flush()
			}
			e.w = w
		}
	}

	if limitBits != 0 {
		limitW := e.w.(*bytes.Buffer)
		if !e.IsClean() {
			return specErrorf("Type %v is not byte aligned", t)
		}
		e.w = wBeforeLimit
		// Write limit
		limit := limitW.Len()
		if err = e.WriteBits(limitBits, uint32(limit)); err != nil {
			return
		}
		// Write content
		if _, err = io.Copy(e.w, limitW); err != nil {
			return
		}
	}
	return
}

var errDecodeEOFBeforeArraySlice = errors.New("EOF before reading slice")
var errEncodeEmptySliceArrayOmitted = errors.New("Empty slice array omitted")

func (d *Decoder) decodeSlice(v reflect.Value, fi *fieldInfo) (err error) {
	// Read length
	var len uint32
	if len, err = d.ReadBits(fi.lenbits); err != nil {
		if readErr, ok := err.(*flate.ReadError); ok && readErr.Err == io.EOF {
			err = errDecodeEOFBeforeArraySlice
		}
		return
	}
	// Read content
	v.SetLen(0)
	var v1 = v
	for i := 0; i < int(len); i++ {
		elem := reflect.New(fi.elemIndirectType)
		// Array element can only be struct currently.
		// fi.encodeElem is always Encoder.encodeStruct.
		if err = fi.decodeElem(d, reflect.Indirect(elem), nil); err != nil {
			return
		}
		if !fi.elemPtr {
			elem = reflect.Indirect(elem)
		}
		v1 = reflect.Append(v1, elem)
	}
	v.Set(v1)
	return
}

func (e *Encoder) encodeSlice(v reflect.Value, fi *fieldInfo) (err error) {
	// Write length
	var len = uint32(v.Len())
	if len == 0 && fi.zlib {
		return errEncodeEmptySliceArrayOmitted
	}
	if err = e.WriteBits(fi.lenbits, len); err != nil {
		return
	}
	// Write content
	for i := 0; i < int(len); i++ {
		elem := v.Index(i)
		if fi.elemPtr {
			if elem.IsNil() {
				return fmt.Errorf("Nil pointer found: %v.%v", fi.structIndirectType, fi.field.Name)
			}
			elem = reflect.Indirect(elem)
		}
		// Array element can only be struct currently.
		// fi.encodeElem is always Encoder.encodeStruct.
		if err = fi.encodeElem(e, elem, nil); err != nil {
			return
		}
	}
	return
}

func (d *Decoder) decodeArray(v reflect.Value, fi *fieldInfo) (err error) {
	// Read length
	var len uint32
	if len, err = d.ReadBits(fi.lenbits); err != nil {
		if readErr, ok := err.(*flate.ReadError); ok && readErr.Err == io.EOF {
			err = errDecodeEOFBeforeArraySlice
		}
		return
	}
	// Read content
	if int(len) > v.Len() {
		return fmt.Errorf("Index out of range when reading %v.%v", fi.structIndirectType, fi.field.Name)
	}
	for i := 0; i < int(len); i++ {
		elem := reflect.New(fi.elemIndirectType)
		// Array element can only be struct currently.
		// fi.encodeElem is always Encoder.encodeStruct.
		if err = fi.decodeElem(d, elem, nil); err != nil {
			return
		}
		if fi.elemPtr {
			elem = elem.Addr()
		}
		v.Index(i).Set(elem)
	}
	return
}

func (e *Encoder) encodeArray(v reflect.Value, fi *fieldInfo) (err error) {
	// Write length
	var len = uint32(v.Len())
	if len == 0 && fi.zlib {
		return errEncodeEmptySliceArrayOmitted
	}
	if err = e.WriteBits(fi.lenbits, len); err != nil {
		return
	}
	// Write content
	for i := 0; i < int(len); i++ {
		elem := v.Index(i)
		if fi.elemPtr {
			if elem.IsNil() {
				return fmt.Errorf("Nil pointer found: %v.%v", fi.structIndirectType, fi.field.Name)
			}
			elem = reflect.Indirect(elem)
		}
		// Array element can only be struct currently.
		// fi.encodeElem is always Encoder.encodeStruct.
		if err = fi.encodeElem(e, elem, nil); err != nil {
			return
		}
	}
	return
}

func (d *Decoder) decodeString(v reflect.Value, fi *fieldInfo) (err error) {
	// Read length
	var len uint32
	if len, err = d.ReadBits(fi.lenbits); err != nil {
		return
	}
	// Read content
	buf := make([]byte, int(len))
	if _, err = io.ReadFull(d, buf); err != nil {
		return
	}
	v.SetString(string(buf))
	return
}

func (e *Encoder) encodeString(v reflect.Value, fi *fieldInfo) (err error) {
	// Write length
	var len = uint32(v.Len())
	if err = e.WriteBits(fi.lenbits, len); err != nil {
		return
	}
	// Write content
	buf := []byte(v.String())
	if _, err = e.Write(buf); err != nil {
		return
	}
	return
}

func (d *Decoder) decodeInt(v reflect.Value, fi *fieldInfo) (err error) {
	var value uint32
	if value, err = d.ReadBits(fi.bits); err != nil {
		return
	}
	v.SetInt(int64(value))
	return
}

func (e *Encoder) encodeInt(v reflect.Value, fi *fieldInfo) (err error) {
	var value = uint32(v.Int())
	if err = e.WriteBits(fi.bits, value); err != nil {
		return
	}
	return
}

func (d *Decoder) decodeUint(v reflect.Value, fi *fieldInfo) (err error) {
	var value uint32
	if value, err = d.ReadBits(fi.bits); err != nil {
		return
	}
	v.SetUint(uint64(value))
	return
}

func (e *Encoder) encodeUint(v reflect.Value, fi *fieldInfo) (err error) {
	var value = uint32(v.Uint())
	if err = e.WriteBits(fi.bits, value); err != nil {
		return
	}
	return
}
