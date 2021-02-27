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
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/hannibal/appconf"
)

var ErrInvalidSignature = errors.New("invalid signature")
var ErrInvalidPackage = errors.New("invalid package")

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

	configPath := filepath.Join(projectPath, "config")
	appConfig, err := appconf.Load(configPath)
	if err != nil {
		return err
	}

	if appConfig.Deploy != nil && appConfig.Deploy.ExecLocal != nil {
		execLocal := appConfig.Deploy.ExecLocal
		cmd := exec.CommandContext(ctx, execLocal.Cmd, execLocal.Args...)
		cmd.Dir = projectPath
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("deploy.exec-local failed: %w", err)
		}
	}

	var ignorePaths []string
	if appConfig.Deploy != nil {
		ignorePaths = appConfig.Deploy.IgnorePaths
	}

	pkgSrc, err := newPackageWriter(projectPath, ignorePaths)
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

		hashDigest := sha512.New()
		digestingWriter := io.MultiWriter(partWriter, hashDigest)

		_, err = pkgSrc.WriteTo(digestingWriter)
		if err != nil {
			wp.CloseWithError(fmt.Errorf("failed to write package part: %w", err))
			return
		}

		digest := hashDigest.Sum(nil)
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

func newPackageWriter(pkgPath string, ignorePaths []string) (*packageWriter, error) {
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

		for _, ignorePath := range ignorePaths {
			if strings.HasPrefix(pkgRelPath, ignorePath) {
				return nil
			}
		}

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
// pkg with one of keys.
func Unpack(pkg io.ReadSeeker, sig []byte, path string, keys []ed25519.PublicKey) error {
	if validSignature, err := isValidSignature(pkg, sig, keys); err == nil {
		if !validSignature {
			return ErrInvalidSignature
		}
	} else {
		return err
	}

	_, err := pkg.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return decompressPackage(pkg, path)
}

func isValidSignature(pkg io.Reader, sig []byte, keys []ed25519.PublicKey) (bool, error) {
	hashDigest := sha512.New()
	_, err := io.Copy(hashDigest, pkg)
	if err != nil {
		return false, err
	}
	digest := hashDigest.Sum(nil)

	for _, k := range keys {
		if ed25519.Verify(k, digest, sig) {
			return true, nil
		}
	}

	return false, nil
}

func decompressPackage(pkg io.Reader, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	_, err = os.Stat(path)
	if err != nil {
		return err
	}

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

		filename := filepath.Join(path, header.Name)

		// Check for directory traversal.
		if !strings.HasPrefix(filename, path) {
			return ErrInvalidPackage
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err := os.Mkdir(filename, os.FileMode(header.Mode&0777))
			if err != nil {
				return err
			}
		case tar.TypeReg:
			err := func() (_err error) {
				file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode&0777))
				if err != nil {
					return err
				}
				defer func() {
					err := file.Close()
					if err != nil && _err == nil {
						_err = err
					}
				}()

				_, err = io.Copy(file, tr)
				if err != nil {
					return err
				}

				return nil
			}()
			if err != nil {
				return err
			}
		default:
			return ErrInvalidPackage
		}
	}

	return nil
}
