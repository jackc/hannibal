package deploy

import (
	"archive/tar"
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

type PackageFile struct {
	Path      string
	tarHeader *tar.Header
}

type Package struct {
	Path  string
	Files []PackageFile
}

func NewPackage(pkgPath string) (*Package, error) {
	pkg := &Package{Path: pkgPath}

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

		pkg.Files = append(pkg.Files, PackageFile{
			Path:      pkgRelPath,
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

func (pkg *Package) WriteTo(w io.Writer) (int64, error) {
	var bytesWritten int64
	w = io.MultiWriter(w, (*lenWriter)(&bytesWritten))

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	for _, pkgFile := range pkg.Files {
		err := tw.WriteHeader(pkgFile.tarHeader)
		if err != nil {
			return bytesWritten, fmt.Errorf("failed to write tar header for %s: %v", pkgFile.Path, err)
		}

		if pkgFile.tarHeader.Typeflag != tar.TypeDir {
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

type Deployer struct {
	URL       string
	APIKey    string
	DeployKey string
}

func (d *Deployer) Deploy(ctx context.Context, pkg *Package) error {
	deployKeySeed, err := hex.DecodeString(d.DeployKey)
	if err != nil {
		return fmt.Errorf("deploy key must be hex encoded: %w", err)
	}
	if len(deployKeySeed) != ed25519.SeedSize {
		return fmt.Errorf("deploy key must have length %d", hex.EncodedLen(ed25519.SeedSize))
	}
	privateKey := ed25519.NewKeyFromSeed(deployKeySeed)

	rp, wp := io.Pipe()
	mw := multipart.NewWriter(wp)

	url, err := urlpkg.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("invalid deploy url: %w", err)
	}
	url.Path = path.Join(url.Path, "hannibal-system/deploy")

	request, err := http.NewRequestWithContext(
		ctx,
		"POST",
		url.String(),
		rp,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Add("Content-Type", mw.FormDataContentType())
	request.Header.Add("Authorization", fmt.Sprintf("%s %s", "hannibal", d.APIKey))

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

		_, err = pkg.WriteTo(digestingWriter)
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

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		respBody, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("HTTP %d %s", response.StatusCode, string(respBody))
	}

	return nil
}
