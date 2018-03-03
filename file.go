package goforit

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const DefaultRefreshInterval = 30 * time.Second

// Identify losing too many flags at once
const shrinkMinFraction = 0.8
const shrinkMaxAmount = 10

// A FileFormat knows how to read a file format
type FileFormat interface {
	// Read reads flags from the given file
	// It yields a list of flags and the last modified time, if it's determinable from the file contents.
	// Returns empty time if it's unknown.
	Read(io.Reader) ([]Flag, time.Time, error)
}

// ErrFileMissing indicates a file that isn't present
type ErrFileMissing struct {
	Path string
}

// Error yields the error message for ErrFileMissing
func (e ErrFileMissing) Error() string {
	return fmt.Sprintf("Missing flag file: file=%s", e.Path)
}

func (e ErrFileMissing) IsCritical() bool {
	return true
}

// ErrFileFormat indicates an error parsing a file
type ErrFileFormat struct {
	Path  string
	Cause error
}

// Error yields the error message for ErrFileFormat
func (e ErrFileFormat) Error() string {
	return fmt.Sprintf("Error parsing flag file: file=%s: err=%s", e.Path, e.Cause.Error())
}

func (e ErrFileFormat) IsCritical() bool {
	return true
}

// ErrFlagsShrunk indicates that the number of flags decreased by an unlikely amount.
// We'll still believe the shrink, but we're cautious that we may be missing flags.
type ErrFlagsShrunk struct {
	Old int
	New int
}

// Error yields the error message for ErrFlagsShrunk
func (e ErrFlagsShrunk) Error() string {
	return fmt.Sprintf("Flag count shrunk by an unlikely amount: old=%d: new=%d", e.Old, e.New)
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

// Refresh our data
func (fb *fileBackend) refresh() error {
	// Open our file
	f, err := os.Open(fb.path)
	if os.IsNotExist(err) {
		return fb.handleError(ErrFileMissing{fb.path})
	} else if err != nil {
		return fb.handleError(err)
	}
	defer f.Close()

	// Read and parse our data
	flags, lastMod, err := fb.format.Read(f)
	if err != nil {
		return fb.handleError(ErrFileFormat{fb.path, err})
	}

	// Get a last-modified date from the file itself, if we don't have one in the data
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

	// Swap old and new
	fb.mtx.Lock()
	defer fb.mtx.Unlock()
	fb.lastMod = lastMod
	old := fb.flags
	fb.flags = fmap

	// Handle the new age info
	if !lastMod.IsZero() {
		fb.handleAge(time.Now().Sub(lastMod))
	}
	// Check if we shrunk by an alarming amount
	if len(old)-len(fmap) > shrinkMaxAmount && float64(len(fmap))/float64(len(old)) < shrinkMinFraction {
		return fb.handleError(ErrFlagsShrunk{len(old), len(fmap)})
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
