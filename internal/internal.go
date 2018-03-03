package internal

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Write a file atomically, for testing purposes
func AtomicWriteFile(t *testing.T, f *os.File, s string) {
	tmp, err := ioutil.TempFile(filepath.Dir(f.Name()), "goforit-")
	require.NoError(t, err)
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	_, err = tmp.WriteString(s)
	require.NoError(t, err)
	err = tmp.Close()
	require.NoError(t, err)
	err = os.Rename(tmp.Name(), f.Name())
	require.NoError(t, err)
}
