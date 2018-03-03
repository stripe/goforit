package goforit

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"
)

// CsvFile format knows how to read CSV files
type CsvFileFormat struct{}

// Read reads flags from a CSV file
func (f CsvFileFormat) Read(r io.Reader) ([]Flag, time.Time, error) {
	// Every row should have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	var rows [][]string
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, time.Time{}, err
	}

	var flags []Flag
	for _, row := range rows {
		name := row[0]
		rate, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, time.Time{}, err
		}

		flags = append(flags, SampleFlag{FlagName: name, Rate: rate})
	}
	return flags, time.Time{}, nil
}

func NewCsvBackend(path string, refreshInterval time.Duration) Backend {
	return NewFileBackend(path, CsvFileFormat{}, refreshInterval)
}
