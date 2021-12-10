package burrow

import (
	"io/fs"
	"net/http"
)

// Type Dir is an enhanced version of http.Dir.
type Dir struct {
	http.Dir
	// AllowListDir indicates whether dir listing is allowed.
	AllowListDir bool
}

func (fs *Dir) Open(name string) (f http.File, err error) {
	f, err = fs.Dir.Open(name)
	if err != nil {
		return
	}
	return &dirFile{f, fs.AllowListDir}, nil
}

type dirFile struct {
	http.File
	AllowListDir bool
}

func (f *dirFile) Readdir(count int) (fi []fs.FileInfo, err error) {
	if !f.AllowListDir {
		return nil, nil
	}
	return f.File.Readdir(count)
}
