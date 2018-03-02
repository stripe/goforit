package refactor

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const DefaultRefreshInterval = 30 * time.Second

// A FileFormat knows how to read a file format
type FileFormat interface {
	// Read reads flags from the given file
	// It yields a list of flags and the last modified time.
	// Returns empty time if it's unknown.
	Read(io.Reader) ([]Flag, time.Time, error)
}

// ErrFileMissing indicates a file that isn't present
type ErrFileMissing struct {
	Path string
}

// Error yields the error message for ErrFileMissing
func (e ErrFileMissing) Error() string {
	return fmt.Sprintf("Missing flag file: %s", e.Path)
}

// ErrFileFormat indicates an error parsing a file
type ErrFileFormat struct {
	Path  string
	Cause error
}

// Error yields the error message for ErrFileFormat
func (e ErrFileFormat) Error() string {
	return fmt.Sprintf("Error parsing flag file %s: %s", e.Path, e.Cause.Error())
}

// fileBackend is a backend based on a file
type fileBackend struct {
	BackendBase

	mtx     sync.RWMutex
	flags   map[string]Flag
	lastMod time.Time

	path            string
	format          FileFormat
	refreshInterval time.Duration
	ticker          *time.Ticker
}

func (fb *fileBackend) refresh() error {
	f, err := os.Open(fb.path)
	if os.IsNotExist(err) {
		return fb.handleError(ErrFileMissing{fb.path})
	} else if err != nil {
		return fb.handleError(err)
	}
	defer f.Close()

	flags, lastMod, err := fb.format.Read(f)
	if err != nil {
		return fb.handleError(ErrFileFormat{fb.path, err})
	}

	// Get a last-modified date from the file itself
	if lastMod.IsZero() {
		stat, err := f.Stat()
		if err != nil {
			return fb.handleError(err)
		}
		lastMod = stat.ModTime()
	}

	// Turn the flags into a map
	fmap := map[string]Flag{}
	for _, f := range flags {
		fmap[f.Name()] = f
	}

	fb.mtx.Lock()
	defer fb.mtx.Unlock()
	fb.lastMod = lastMod
	fb.flags = fmap

	if !lastMod.IsZero() {
		fb.handleAge(time.Now().Sub(lastMod))
	}
	return nil
}

func (fb *fileBackend) Close() error {
	fb.ticker.Stop()
	return fb.BackendBase.Close()
}

func (b *fileBackend) SetErrorHandler(h ErrorHandler) {
	b.BackendBase.SetErrorHandler(h)
	// Force a refresh, to catch any errors
	b.refresh()
}

func (fb *fileBackend) Flag(name string) (Flag, time.Time, error) {
	fb.mtx.RLock()
	defer fb.mtx.RUnlock()
	if fb.flags == nil {
		return nil, fb.lastMod, nil
	}
	return fb.flags[name], fb.lastMod, nil
}

// NewFileBackend builds a backend based on a file
func NewFileBackend(path string, format FileFormat, refreshInterval time.Duration) Backend {
	fb := &fileBackend{
		path:            path,
		format:          format,
		refreshInterval: refreshInterval,
		ticker:          time.NewTicker(refreshInterval),
	}
	fb.refresh()
	go func() {
		for range fb.ticker.C {
			fb.refresh()
		}
	}()
	return fb
}
