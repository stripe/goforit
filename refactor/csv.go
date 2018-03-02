package refactor

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"
)

// A FileFormat knows how to read a file format
type csvFileFormat struct{}

func (f csvFileFormat) Read(r io.Reader) (flags []Flag, age time.Time, err error) {
	// Every row should have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	rows, err := cr.ReadAll()
	if err != nil {
		return nil, time.Time{}, err
	}

	var rate float64
	for _, row := range rows {
		name := row[0]
		rate, err = strconv.ParseFloat(row[1], 64)
		if err != nil {
			return
		}

		flags = append(flags, SampleFlag{FlagName: name, Rate: rate})
	}
	return
}
