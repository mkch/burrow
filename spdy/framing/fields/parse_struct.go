package fields

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Shared by all Encoder and Decoder objects.
var parsedStructs = make(structs)
var parsedStructsLock sync.RWMutex

func parseStruct(structType reflect.Type) (si structInfo, err error) {
	var exists bool
	parsedStructsLock.RLock()
	if si, exists = parsedStructs[structType]; exists {
		parsedStructsLock.RUnlock()
		return
	}
	parsedStructsLock.RUnlock()
	// !! RACE, double check needed.
	parsedStructsLock.Lock()
	defer parsedStructsLock.Unlock()
	// Double check.
	if si, exists = parsedStructs[structType]; exists {
		return
	}
	return parsedStructs.parse(structType, nil)
}

type DecodeFunc func(*Decoder, reflect.Value, *fieldInfo) error
type EncodeFunc func(*Encoder, reflect.Value, *fieldInfo) error

// Infmormation of a struct field.
type fieldInfo struct {
	// Parsed from the struct tag.
	bits    int
	lenbits int
	limit   bool
	zlib    bool
	// Additional information of this field.
	decode             DecodeFunc // The function to decode this field.
	encode             EncodeFunc
	decodeElem         DecodeFunc // The function to decode the element of this field. For slice, array only.
	encodeElem         EncodeFunc
	structIndirectType reflect.Type        // The indirect type(never be ptr type) of thie struct this field belongs
	field              reflect.StructField // This field.
	indirectType       reflect.Type        // The indirect type(never be ptr type) of this field.
	ptr                bool                // Whether this field is ptr type.
	elemPtr            bool                // Whether the element of this field is ptr. For slice, array only.
	elemIndirectType   reflect.Type        // The indirect type(never be ptr type) of the element. For slice, array only.
}

// Information of all fields in a struct.
type structInfo []*fieldInfo

// struct -> all fields.
type structs map[reflect.Type]structInfo

// Parse parses a struct type to get the field information
func (m structs) Parse(structType reflect.Type) (si structInfo, err error) {
	var exists bool
	if si, exists = m[structType]; exists {
		return
	}
	return m.parse(structType, nil)
}

type parseRouteNode struct {
	structType reflect.Type
	fieldName  string
}

func (n *parseRouteNode) String() string {
	return fmt.Sprintf("%v.%v", n.structType, n.fieldName)
}

// parse parses a struct type t
func (m structs) parse(t reflect.Type, seen []*parseRouteNode) (info structInfo, err error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Circular type.
	for i, node := range seen {
		if node.structType == t {
			var ts []string
			for i2 := i; i2 < len(seen); i2++ {
				ts = append(ts, seen[i2].String())
			}
			ts = append(ts, t.String())
			return nil, specErrorf("Circular type %v:\n\t%v", t, strings.Join(ts, " -> "))
		}
	}

	var si structInfo
	var limited bool
	var totalBits int
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldType := field.Type
		var ptr bool
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
			ptr = true
		}
		// Parse tag.
		tag := field.Tag.Get("field")
		if tag == "" && fieldType.Kind() != reflect.Struct {
			return nil, specErrorf("field %v.%v is untagged", t, field.Name)
		}
		if tag == "-" {
			si = append(si, nil)
			continue
		}
		var fi *fieldInfo
		if fi, err = parseTag(t, field.Name, tag); err != nil {
			return
		}
		if fi.limit {
			// Check duplicated "limit".
			if limited {
				return nil, specErrorf(`Duplicated spec "limit" on struct %v`, t)
			} else {
				limited = true
			}
		}
		fi.ptr = ptr
		fi.indirectType = fieldType
		fi.field = field
		fi.structIndirectType = t
		seen = append(seen, &parseRouteNode{t, field.Name})
		// Check type.
		switch fieldType.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint, reflect.Uint32, reflect.Uint64:
			fi.decode = (*Decoder).decodeUint
			fi.encode = (*Encoder).encodeUint
			if fi.zlib {
				return nil, specErrorf(`Spec "zlib" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.lenbits != 0 {
				return nil, specErrorf(`Spec "lenbits" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
		case reflect.Int8, reflect.Int16, reflect.Int, reflect.Int32, reflect.Int64:
			if fi.limit {
				return nil, specErrorf(`Spec "limit" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.lenbits != 0 {
				return nil, specErrorf(`Spec "lenbits" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.zlib {
				return nil, specErrorf(`Spec "zlib" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			fi.decode = (*Decoder).decodeInt
			fi.encode = (*Encoder).encodeInt
		case reflect.String:
			if fi.limit {
				return nil, specErrorf(`Spec "limit" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.zlib {
				return nil, specErrorf(`Spec "zlib" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.lenbits == 0 {
				return nil, specErrorf(`Spec "lenbits" is required for type %v (%v.%v)`, fieldType, t, field.Name)
			}
			fi.decode = (*Decoder).decodeString
			fi.encode = (*Encoder).encodeString
		case reflect.Array:
			fi.decode = (*Decoder).decodeArray
			fi.encode = (*Encoder).encodeArray
			fallthrough
		case reflect.Slice:
			if fi.bits != 0 {
				return nil, specErrorf(`Spec "bits" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.limit {
				return nil, specErrorf(`Spec "limit" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.lenbits == 0 {
				return nil, specErrorf(`Spec "lenbits" is required for type %v (%v.%v)`, fieldType, t, field.Name)
			}
			elemType := fieldType.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
				fi.elemPtr = true
			}
			fi.elemIndirectType = elemType
			switch elemType.Kind() {
			case reflect.Struct:
				if _, exists := m[elemType]; !exists {
					if _, err = m.parse(elemType, seen); err != nil {
						return
					}
				}
				fi.decodeElem = (*Decoder).decodeStruct
				fi.encodeElem = (*Encoder).encodeStruct
			default:
				return nil, specErrorf("Unsupported type %v (%v.%v)", fieldType, t, field.Name)
			}
			if fi.decode == nil {
				fi.decode = (*Decoder).decodeSlice
			}
			if fi.encode == nil {
				fi.encode = (*Encoder).encodeSlice
			}
		case reflect.Struct:
			if fi.bits != 0 {
				return nil, specErrorf(`Spec "bits" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.limit {
				return nil, specErrorf(`Spec "limit" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.lenbits != 0 {
				return nil, specErrorf(`Spec "lenbits" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if fi.zlib {
				return nil, specErrorf(`Spec "zilb" comes with wrong type %v (%v.%v)`, fieldType, t, field.Name)
			}
			if _, exists := m[fieldType]; !exists {
				if _, err = m.parse(fieldType, seen); err != nil {
					return
				}
			}
			fi.decode = (*Decoder).decodeStruct
			fi.encode = (*Encoder).encodeStruct
		default:
			return nil, specErrorf("Unsupported type %v (%v.%v)", fieldType, t, field.Name)
		}
		if fi.limit {
			// Check struct byte-alignment
			if totalBits%8 != 0 {
				return nil, specErrorf(`Struct %v is not byte-aligned before field %v which is tagged by "limit"`, t, field.Name)
			}
			// Check limit byte-alignment
			if fi.bits != 0 && fi.bits%8 != 0 {
				return nil, specErrorf(`"bits" value of "limit"ed field %v.%v is not byte-aligned`, t, field.Name)
			}
		}
		if fi.zlib {
			// Check struct byte-alignment
			if totalBits%8 != 0 {
				return nil, specErrorf(`Struct %v is not byte-aligned before field %v which is tagged by "zlib"`, t, field.Name)
			}
		}
		totalBits += fi.bits
		si = append(si, fi)
	}
	// Check struct byte-alignment
	if totalBits%8 != 0 {
		return nil, specErrorf(`Struct %v is not byte-aligned`, t)
	}

	var z bool
	for i := len(si) - 1; i >= 0; i-- {
		if si[i] != nil {
			if si[i].zlib {
				if z {
					return nil, specErrorf(`Spec "zlib" can only applied to the last slice field of a struct. %v.%v`, t, si[i].field.Name)
				}
				z = true
			}
		}
	}
	if z && !limited {
		return nil, specErrorf(`Spec "zlib" needs a "limit" field in struct %v`, t)
	}

	m[t] = si
	return si, nil
}

func parseTag(t reflect.Type, f string, tag string) (info *fieldInfo, err error) {
	var fi fieldInfo
	for _, spec := range strings.Split(tag, ",") {
		nv := strings.Split(spec, ":")
		if len(nv) > 2 {
			return nil, specErrorf("Invalid tag: %v", tag)
		}
		var key = nv[0]
		var value *string
		if len(nv) == 2 {
			v := nv[1]
			value = &v
		}
		switch key {
		case "bits":
			if fi.bits != 0 {
				return nil, specErrorf(`Duplicated spec "bits" on %v.%v`, t, f)
			}
			if value == nil {
				return nil, specErrorf(`Spec "bits" on %v.%v has no value`, t, f)
			}
			bits, err := strconv.Atoi(*value)
			if err != nil || bits <= 0 || bits > 32 {
				return nil, specErrorf(`Spec "bits" on %v.%v has invalid value %v`, t, f, *value)
			}
			fi.bits = bits
		case "limit":
			if fi.limit {
				return nil, specErrorf(`Duplicated spec "limit" on %v.%v`, t, f)
			}
			if value != nil {
				return nil, specErrorf(`Unnecessary value of spec "limit" on %v.%v`, t, f)
			}
			fi.limit = true
		case "lenbits":
			if fi.lenbits != 0 {
				return nil, specErrorf(`Duplicated spec "lenbits" on %v.%v`, t, f)
			}
			if value == nil {
				return nil, specErrorf(`Spec "lenbits" on %v.%v has no value`, t, f)
			}
			lenbits, err := strconv.Atoi(*value)
			if err != nil || lenbits <= 0 || lenbits > 32 {
				return nil, specErrorf(`Spec "lenbits" on %v.%v has invalid value %v`, t, f, *value)
			}
			if lenbits%8 != 0 {
				return nil, specErrorf(`"lenbits" value %v on %v.%v is not multiple of 8`, lenbits, t, f)
			}
			fi.lenbits = lenbits
		case "zlib":
			if fi.limit {
				return nil, specErrorf(`Duplicated spec "zlib" on %v.%v`, t, f)
			}
			if value != nil {
				return nil, specErrorf(`Unnecessary value of spec "zlib" on %v.%v`, t, f)
			}
			fi.zlib = true
		}
	}
	return &fi, nil
}
