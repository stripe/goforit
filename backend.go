package goforit

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/stripe/goforit/flags2"
)

type Backend interface {
	// Refresh returns a new set of flags.
	// It also returns the age of these flags, or an empty time if no age is known.
	Refresh() ([]*flags2.Flag2, time.Time, error)
}

type jsonFileBackend2 struct {
	filename string
}

func readFile(file string, parse func(io.Reader) ([]*flags2.Flag2, time.Time, error)) ([]*flags2.Flag2, time.Time, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	return parse(bufio.NewReaderSize(f, 128*1024))
}

func (b jsonFileBackend2) Refresh() ([]*flags2.Flag2, time.Time, error) {
	flags, updated, err := readFile(b.filename, parseFlagsJSON2)
	if updated != time.Unix(0, 0) {
		return flags, updated, err
	}

	fileInfo, err := os.Stat(b.filename)
	if err != nil {
		return nil, time.Time{}, err
	}
	return flags, fileInfo.ModTime(), nil
}

func parseFlagsJSON2(r io.Reader) ([]*flags2.Flag2, time.Time, error) {
	dec := json.NewDecoder(r)
	var v flags2.JSONFormat2
	err := dec.Decode(&v)
	if err != nil {
		return nil, time.Time{}, err
	}

	flags := make([]*flags2.Flag2, len(v.Flags))
	for i, f := range v.Flags {
		flags[i] = f
	}

	return flags, time.Unix(int64(v.Updated), 0), nil
}

// BackendFromJSONFile2 creates a v2 backend powered by a JSON file
func BackendFromJSONFile2(filename string) Backend {
	return jsonFileBackend2{filename}
}
