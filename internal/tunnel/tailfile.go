package tunnel

import (
	"io"
	"os"
	"sync"
	"time"
)

// tailReader reads a file that another process is still writing (the macOS
// helper's output log): at the end of the file it waits for more data, and
// returns io.EOF once done is closed and everything has been read. The file
// is closed at that point (or on the first error), so the reader must be read
// until it fails.
type tailReader struct {
	f    *os.File
	done <-chan struct{}
	once sync.Once
}

func openTail(path string, done <-chan struct{}) (*tailReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &tailReader{f: f, done: done}, nil
}

func (r *tailReader) Read(p []byte) (int, error) {
	n, err := r.read(p)
	if err != nil {
		r.Close()
	}
	return n, err
}

// Close releases the file; it is safe to call more than once.
func (r *tailReader) Close() error {
	var err error
	r.once.Do(func() { err = r.f.Close() })
	return err
}

func (r *tailReader) read(p []byte) (int, error) {
	for {
		n, err := r.f.Read(p)
		if n > 0 || (err != nil && err != io.EOF) {
			return n, err
		}
		select {
		case <-r.done:
			// the writer has finished: what it wrote before is visible now
			n, err := r.f.Read(p)
			if n > 0 || (err != nil && err != io.EOF) {
				return n, err
			}
			return 0, io.EOF
		case <-time.After(200 * time.Millisecond):
		}
	}
}
