package upgradecmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errBinaryNotFound = fmt.Errorf("%s binary not found in archive", upgradeBinaryName)

// releaseArchiveName returns the release asset name for a platform, mirroring
// .goreleaser.yml: name_template plus format_overrides (Windows ships .zip,
// every other OS ships .tar.gz).
func releaseArchiveName(versionNum, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s-%s-%s-%s%s", upgradeBinaryName, versionNum, goos, goarch, ext)
}

// extractBinary copies the openbee binary out of archivePath into dest,
// choosing the archive format from the file extension.
func extractBinary(archivePath string, dest io.Writer) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, dest)
	}
	return extractFromTarGz(archivePath, dest)
}

func extractFromTarGz(archivePath string, dest io.Writer) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || !isUpgradeBinary(hdr.Name) {
			continue
		}
		_, err = io.Copy(dest, tr)
		return err
	}
	return errBinaryNotFound
}

func extractFromZip(archivePath string, dest io.Writer) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if !zf.Mode().IsRegular() || !isUpgradeBinary(zf.Name) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		_, err = io.Copy(dest, rc)
		return err
	}
	return errBinaryNotFound
}

func isUpgradeBinary(name string) bool {
	base := filepath.Base(name)
	return base == upgradeBinaryName || base == upgradeBinaryNameWin
}
