package framing

import (
	"testing"
)

func TestNameBlockV2InsertDelete(t *testing.T) {
	t.Parallel()

	// Test insert
	var b headerBlockV2
	b.insert(0, nameValueV2{"a", "b"})
	if len(b) != 1 || b[0].Name != "a" || b[0].Value != "b" {
		t.Fatal()
	}
	b.insert(0, nameValueV2{"c", "d"})
	b.insert(2, nameValueV2{"e", "f"})
	b.insert(1, nameValueV2{"g", "h"})
	if len(b) != 4 || b[0].Name != "c" || b[1].Name != "g" || b[2].Name != "a" || b[3].Name != "e" {
		t.Fatal()
	}

	b.delete(0)
	if len(b) != 3 || b[0].Name != "g" {
		t.Fatal()
	}

	b.delete(2)
	if len(b) != 2 || b[1].Name != "a" {
		t.Fatal()
	}

}

func TestNameBlockV2AddGetNames(t *testing.T) {
	t.Parallel()

	var b headerBlockV2
	var err error

	if err = b.Add("k1", "v1"); err != nil ||
		len(b) != 1 || b[0].Name != "k1" || b[0].Value != "v1" {
		t.Fatalf("err=%v b=%v", err, b)
	}

	if err = b.Add("c1", "v2"); err != nil ||
		len(b) != 2 ||
		b[0].Name != "c1" || b[0].Value != "v2" ||
		b[1].Name != "k1" || b[1].Value != "v1" {
		t.Fatalf("err=%v b=%v", err, b)
	}

	if err = b.Add("d1", "v3"); err != nil ||
		len(b) != 3 ||
		b[0].Name != "c1" || b[0].Value != "v2" ||
		b[1].Name != "d1" || b[1].Value != "v3" ||
		b[2].Name != "k1" || b[2].Value != "v1" {
		t.Fatalf("err=%v b=%v", err, b)
	}

	if err = b.Add("d1", "v4", "v5"); err != nil ||
		len(b) != 3 ||
		b[0].Name != "c1" || b[0].Value != "v2" ||
		b[1].Name != "d1" || b[1].Value != "v3\x00v4\x00v5" ||
		b[2].Name != "k1" || b[2].Value != "v1" {
		t.Fatalf("err=%v b=%v", err, b)
	}

	if names := b.Names(); len(names) != 3 || names[0] != "c1" || names[1] != "d1" ||
		names[2] != "k1" {
		t.Fatal(names)
	}

	if v := b.GetFirst("D1"); v != "v3" {
		t.Fatal(v)
	}

	if vs := b.Get("D1"); len(vs) != 3 || vs[0] != "v3" || vs[1] != "v4" || vs[2] != "v5" {
		t.Fatal(vs)
	}

	if v := b.GetFirst("no-this-name"); v != "" {
		t.Fatal(v)
	}

	if vs := b.Get("no-this-name"); len(vs) != 0 {
		t.Fatal(vs)
	}

	if vs := b.Get("c1"); len(vs) != 1 || vs[0] != "v2" {
		t.Fatal(vs)
	}
}
