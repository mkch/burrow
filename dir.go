package burrow

import (
	"net/http"
	"os"
	"path"
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
	if fs.AllowListDir {
		return
	}

	var fileInfo os.FileInfo
	if fileInfo, err = f.Stat(); err != nil {
		return nil, err
	}
	fileInfo.Mode()
	if fileInfo.IsDir() {
		index := path.Join(name, "index.html")
		f, err = fs.Dir.Open(index)
	}
	return
}
