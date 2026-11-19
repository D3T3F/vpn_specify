package setup

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
)

// sameContent reports whether the file dst on disk has the same size and
// SHA-256 as the file name in src. Any error means the files could not be
// compared, so the caller should treat them as different.
func sameContent(src fs.FS, name, dst string) (bool, error) {
	srcInfo, err := fs.Stat(src, name)
	if err != nil {
		return false, err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return false, err
	}
	if dstInfo.IsDir() { // a directory has no content to compare (its "size" depends on the OS)
		return false, errors.New("destination is a directory")
	}
	if srcInfo.Size() != dstInfo.Size() {
		return false, nil
	}
	srcSum, err := hashFS(src, name)
	if err != nil {
		return false, err
	}
	dstSum, err := hashPath(dst)
	if err != nil {
		return false, err
	}
	return bytes.Equal(srcSum, dstSum), nil
}

// hashFS returns the SHA-256 of the file name in fsys.
func hashFS(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return hashReader(f)
}

// hashPath returns the SHA-256 of the file at path.
func hashPath(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return hashReader(f)
}

func hashReader(r io.Reader) ([]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
