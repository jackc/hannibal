package deploy

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha512"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type lenWriter int64

func (lw *lenWriter) Write(p []byte) (int, error) {
	*lw += lenWriter(len(p))
	return len(p), nil
}

func BuildPackage(w io.Writer, pkgPath string) (int64, []byte, error) {
	hash := sha512.New512_256()
	lenWriter := new(lenWriter)
	multiWriter := io.MultiWriter(w, hash, lenWriter)

	gw := gzip.NewWriter(multiWriter)
	tw := tar.NewWriter(gw)

	walkFunc := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk for %s: %v", path, walkErr)
		}

		pkgRelPath := path[len(pkgPath):]
		if pkgRelPath == "" {
			return nil
		}
		pkgRelPath = pkgRelPath[1:] // remove "/"
		fmt.Println(pkgRelPath)

		th := &tar.Header{
			Name: pkgRelPath,
			Mode: int64(info.Mode()),
			Size: info.Size(),
		}

		err := tw.WriteHeader(th)
		if err != nil {
			return fmt.Errorf("failed to write tar header for %s: %v", path, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", path, err)
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		if err != nil {
			return fmt.Errorf("failed to copy to tar %s: %v", path, err)
		}

		return nil
	}

	err := filepath.Walk(pkgPath, walkFunc)
	if err != nil {
		return int64(*lenWriter), hash.Sum(nil), err
	}

	err = tw.Close()
	if err != nil {
		return int64(*lenWriter), hash.Sum(nil), err
	}

	err = gw.Close()
	if err != nil {
		return int64(*lenWriter), hash.Sum(nil), err
	}

	return int64(*lenWriter), hash.Sum(nil), nil
}
