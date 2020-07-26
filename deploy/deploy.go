package deploy

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type PackageFile struct {
	Path string
	Mode os.FileMode
	Size int64
}

type Package struct {
	Path  string
	Files []PackageFile
}

func NewPackage(pkgPath string) (*Package, error) {
	pkg := &Package{Path: pkgPath}

	walkFunc := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk for %s: %v", path, walkErr)
		}

		pkgRelPath := path[len(pkgPath):]
		if pkgRelPath == "" {
			return nil
		}
		pkgRelPath = pkgRelPath[1:] // remove "/"

		pkg.Files = append(pkg.Files, PackageFile{
			Path: pkgRelPath,
			Mode: info.Mode(),
			Size: info.Size(),
		})

		return nil
	}

	err := filepath.Walk(pkgPath, walkFunc)
	if err != nil {
		return nil, err
	}

	return pkg, nil
}

func (pkg *Package) WriteTo(w io.Writer) (int64, error) {
	var bytesWritten int64
	w = io.MultiWriter(w, (*lenWriter)(&bytesWritten))

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	for _, pkgFile := range pkg.Files {
		th := &tar.Header{
			Name: pkgFile.Path,
			Mode: int64(pkgFile.Mode),
			Size: pkgFile.Size,
		}

		err := tw.WriteHeader(th)
		if err != nil {
			return bytesWritten, fmt.Errorf("failed to write tar header for %s: %v", pkgFile.Path, err)
		}

		// Use function to defer file.Close()
		err = func() error {
			file, err := os.Open(filepath.Join(pkg.Path, pkgFile.Path))
			if err != nil {
				return fmt.Errorf("failed to open %s: %v", pkgFile.Path, err)
			}
			defer file.Close()

			_, err = io.Copy(tw, file)
			if err != nil {
				return fmt.Errorf("failed to copy to tar %s: %v", pkgFile.Path, err)
			}

			return nil
		}()
		if err != nil {
			return bytesWritten, err
		}
	}

	err := tw.Close()
	if err != nil {
		return bytesWritten, err
	}

	err = gw.Close()
	if err != nil {
		return bytesWritten, err
	}

	return bytesWritten, nil
}

type lenWriter int64

func (lw *lenWriter) Write(p []byte) (int, error) {
	*lw += lenWriter(len(p))
	return len(p), nil
}
