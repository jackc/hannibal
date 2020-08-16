package server

import (
	"net/http"
	"path/filepath"
)

type publicFileHandler struct {
	fs http.Handler
}

// NewPublicFileHandler returns a handler that serves files from rootPath. This is used instead of http.FileServer()
// because this does not return directory listings. Also, this provides a point to insert custom 404 handling in the
// future.
func NewPublicFileHandler(rootPath string) http.Handler {
	fs := &publicFileSystem{dir: http.Dir(rootPath)}
	return &publicFileHandler{fs: http.FileServer(fs)}
}

func (pfh *publicFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pfh.fs.ServeHTTP(w, r)
}

type publicFileSystem struct {
	dir http.Dir
}

func (fs *publicFileSystem) Open(name string) (file http.File, err error) {
	f, err := fs.dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close() // Ignore error as an error is already being returned.
		}
	}()

	fileInfo, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if fileInfo.IsDir() {
		indexFile, err := fs.Open(filepath.Join(name, "index.html"))
		if err != nil {
			return nil, err
		}
		err = indexFile.Close()
		if err != nil {
			return nil, err
		}
	}

	return f, nil
}
