package fields

import (
	"log"
	"reflect"
	"testing"
)

type A struct {
	b B
}

type B struct {
	c C
}

type C struct {
	b *B
}

func TestParseStruct(t *testing.T) {
	t.Parallel()

	var p = make(structs)
	var si structInfo
	var err error
	_, err = p.Parse(reflect.TypeOf(*new(struct{ a int })))
	if err == nil {
		t.Fatal()
	}
	if testing.Verbose() {
		log.Println(err)
	}

	_, err = p.Parse(reflect.TypeOf(*new(struct {
		a int `field:"lenbits:8"`
	})))
	if err == nil {
		t.Fatal()
	}
	if testing.Verbose() {
		log.Println(err)
	}

	_, err = p.Parse(reflect.TypeOf(*new(struct {
		b int `field:"limit"`
		c int `field:"limit"`
	})))
	if err == nil {
		t.Fatal()
	}
	if testing.Verbose() {
		log.Println(err)
	}

	_, err = p.Parse(reflect.TypeOf(*new(A)))
	if err == nil {
		t.Fatal()
	}
	if testing.Verbose() {
		log.Println(err)
	}

	si, err = p.Parse(reflect.TypeOf(*new(struct {
		v reflect.Value `field:"-"`
		N byte          `field:"bits:8"`
		L uint32        `field:"bits:32,limit"`
		S []struct{}    `field:"lenbits:8,zlib"`
	})))
	if err != nil || len(si) != 4 ||
		si[0] != nil ||
		si[1].bits != 8 || si[1].limit != false || si[1].lenbits != 0 || si[1].zlib != false ||
		si[2].bits != 32 || si[2].limit != true || si[2].lenbits != 0 || si[2].zlib != false ||
		si[3].bits != 0 || si[3].limit != false || si[3].lenbits != 8 || si[3].zlib != true {
		t.Fatalf("\n%#v %v", si, err)
	}

}
