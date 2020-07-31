package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	urlpkg "net/url"
	"os"
	"path"
	"path/filepath"
)

// Deploy deploys the project at projectPath to the server at URL. httpClient allows for customizing TLS behavior. It
// may be nil.
func Deploy(ctx context.Context, url, apiKey, deployKey, projectPath string, httpClient *http.Client) error {
	deployKeySeed, err := hex.DecodeString(deployKey)
	if err != nil {
		return fmt.Errorf("deploy key must be hex encoded: %w", err)
	}
	if len(deployKeySeed) != ed25519.SeedSize {
		return fmt.Errorf("deploy key must have length %d", hex.EncodedLen(ed25519.SeedSize))
	}
	privateKey := ed25519.NewKeyFromSeed(deployKeySeed)

	pkgSrc, err := newPackageWriter(projectPath)
	if err != nil {
		return err
	}

	rp, wp := io.Pipe()
	mw := multipart.NewWriter(wp)

	urlWithPath, err := urlpkg.Parse(url)
	if err != nil {
		return fmt.Errorf("invalid deploy url: %w", err)
	}
	urlWithPath.Path = path.Join(urlWithPath.Path, "hannibal-system/deploy")

	request, err := http.NewRequestWithContext(
		ctx,
		"POST",
		urlWithPath.String(),
		rp,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Add("Content-Type", mw.FormDataContentType())
	request.Header.Add("Authorization", fmt.Sprintf("%s %s", "hannibal", apiKey))

	go func() {
		defer wp.CloseWithError(errors.New("function exited without closing pipe"))

		mimeHeader := make(textproto.MIMEHeader)
		mimeHeader.Set("Content-Disposition", `form-data; name="pkg"; filename="pkg.tar.gz"`)
		mimeHeader.Set("Content-Type", "application/gzip")
		partWriter, err := mw.CreatePart(mimeHeader)
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to create package part: %w", err))
			return
		}

		hashDigest := sha512.New512_256()
		digestingWriter := io.MultiWriter(partWriter, hashDigest)

		_, err = pkgSrc.WriteTo(digestingWriter)
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to write package part: %w", err))
			return
		}

		digest := hashDigest.Sum(nil)
		err = mw.WriteField("digest", hex.EncodeToString(digest))
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to write digest: %w", err))
			return
		}

		signature := ed25519.Sign(privateKey, digest)
		err = mw.WriteField("signature", hex.EncodeToString(signature))
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to write signature: %w", err))
			return
		}

		err = mw.Close()
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to close multipart writer: %w", err))
			return
		}

		wp.Close()
	}()

	if httpClient == nil {
		httpClient = &http.Client{}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		respBody, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("HTTP %d %s", response.StatusCode, string(respBody))
	}

	io.Copy(os.Stdout, response.Body)

	return nil
}

type packageWriterFile struct {
	path      string
	tarHeader *tar.Header
}

type packageWriter struct {
	path  string
	files []packageWriterFile
}

func newPackageWriter(pkgPath string) (*packageWriter, error) {
	pkg := &packageWriter{path: pkgPath}

	hasSQLManifest := false

	walkFunc := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk for %s: %v", path, walkErr)
		}

		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}

		pkgRelPath := path[len(pkgPath):]
		if pkgRelPath == "" {
			return nil
		}
		pkgRelPath = pkgRelPath[1:] // remove "/"

		if !hasSQLManifest && pkgRelPath == "sql/manifest.conf" {
			hasSQLManifest = true
		}

		tarHeader, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		tarHeader.Name = filepath.ToSlash(pkgRelPath)
		if tarHeader.Typeflag == tar.TypeDir {
			tarHeader.Name = tarHeader.Name + "/"
		}

		pkg.files = append(pkg.files, packageWriterFile{
			path:      pkgRelPath,
			tarHeader: tarHeader,
		})

		return nil
	}

	err := filepath.Walk(pkgPath, walkFunc)
	if err != nil {
		return nil, err
	}

	if !hasSQLManifest {
		return nil, fmt.Errorf("not a package")
	}

	return pkg, nil
}

type lenWriter int64

func (lw *lenWriter) Write(p []byte) (int, error) {
	*lw += lenWriter(len(p))
	return len(p), nil
}

func (pkg *packageWriter) WriteTo(w io.Writer) (int64, error) {
	var bytesWritten int64
	w = io.MultiWriter(w, (*lenWriter)(&bytesWritten))

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	for _, pkgFile := range pkg.files {
		err := tw.WriteHeader(pkgFile.tarHeader)
		if err != nil {
			return bytesWritten, fmt.Errorf("failed to write tar header for %s: %v", pkgFile.path, err)
		}

		if pkgFile.tarHeader.Typeflag != tar.TypeDir {
			// Use function to defer file.Close()
			err = func() error {
				file, err := os.Open(filepath.Join(pkg.path, pkgFile.path))
				if err != nil {
					return fmt.Errorf("failed to open %s: %v", pkgFile.path, err)
				}
				defer file.Close()

				_, err = io.Copy(tw, file)
				if err != nil {
					return fmt.Errorf("failed to copy to tar %s: %v", pkgFile.path, err)
				}

				return nil
			}()
			if err != nil {
				return bytesWritten, err
			}
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

// Unpack unpacks pkg to path. It validates that digest matches pkg that sig is the result of signing
// digest with one of keys.
func Unpack(pkg io.ReadSeeker, digest, sig []byte, path string, keys []ed25519.PublicKey) error {
	if !isValidSignature(digest, sig, keys) {
		return errors.New("invalid signature")
	}

	if validDigest, err := isValidDigest(digest, pkg); err == nil {
		if !validDigest {
			return errors.New("invalid digest")
		}
	} else {
		return err
	}

	return decompressPackage(pkg, path)
}

func isValidSignature(digest, sig []byte, keys []ed25519.PublicKey) bool {
	for _, k := range keys {
		if ed25519.Verify(k, digest, sig) {
			return true
		}
	}

	return false
}

func isValidDigest(digest []byte, pkg io.ReadSeeker) (bool, error) {
	hashDigest := sha512.New512_256()
	_, err := io.Copy(hashDigest, pkg)
	if err != nil {
		return false, err
	}
	pkgDigest := hashDigest.Sum(nil)

	valid := bytes.Compare(digest, pkgDigest) == 0

	_, err = pkg.Seek(0, io.SeekStart)
	return valid, err
}

func decompressPackage(pkg io.Reader, path string) error {
	gr, err := gzip.NewReader(pkg)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			fmt.Println("dir", header.Name)
		case tar.TypeReg:
			fmt.Println("reg", header.Name)
		default:
			return errors.New("invalid package")
		}
	}

	return nil
}
