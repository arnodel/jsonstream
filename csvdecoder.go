package jsonstream

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// A CSVDecoder reads CSV input and streams it into a JSON stream.
type CSVDecoder struct {
	reader                *csv.Reader
	HasHeader             bool // When true, treat the first record as a header
	RecordsProduceObjects bool // When false, produce an array for each record, else an object
	fieldNames            []*token.Scalar
}

var _ token.StreamSource = &CSVDecoder{}

// NewCSVDecoder sets up a new CSVDecoder isntance to read from the given input.
func NewCSVDecoder(in io.Reader) *CSVDecoder {
	return &CSVDecoder{reader: csv.NewReader(in)}
}

// Produce reads a stream of CSV records, until it runs out of input or
// encounters invalid CSV, in which case it will return an error
func (d *CSVDecoder) Produce(out chan<- token.Token) error {
	recordCount := 0
	for {
		record, err := d.reader.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if recordCount > 0 || !d.HasHeader {
			d.produceRecord(record, out)
		} else {
			// Try and get field names from the first record
			d.SetFieldNames(record)
		}
		recordCount++
	}
}

// SetFieldNames sets the field namas for records.  Should be called before Produce.
func (d *CSVDecoder) SetFieldNames(record []string) {
	for _, field := range record {
		d.fieldNames = append(d.fieldNames, fieldToScalar(field, true))
	}
}

func (d *CSVDecoder) produceRecord(record []string, out chan<- token.Token) {
	if d.RecordsProduceObjects {
		out <- &token.StartObject{}
		for i, field := range record {
			out <- d.getFieldName(i)
			out <- fieldToScalar(field, false)
		}
		out <- &token.EndObject{}
	} else {
		out <- &token.StartArray{}
		for _, field := range record {
			out <- fieldToScalar(field, false)
		}
		out <- &token.EndArray{}
	}
}

func (d *CSVDecoder) getFieldName(i int) *token.Scalar {
	if i >= len(d.fieldNames) {
		for j := len(d.fieldNames); j <= i; j++ {
			d.fieldNames = append(d.fieldNames, fieldToScalar(fmt.Sprintf("field_%d", j+1), true))
		}
	}
	return d.fieldNames[i]
}

func fieldToScalar(field string, isHeader bool) *token.Scalar {
	if !isHeader {
		switch field {
		case "":
			return nullInstance
		case "true":
			return trueInstance
		case "false":
			return falseInstance
		}
	}
	var fieldIsAlnum = true
	var fieldCouldBeNumber = !isHeader
	var escapeCount = 0
	for i, b := range []byte(field) {
		if b == '"' || b == '\n' || b == '\\' {
			escapeCount++
		} else if i == 0 {
			fieldIsAlnum = isalpha(b)
		} else {
			fieldIsAlnum = fieldIsAlnum && isalnum(b)
		}
		if fieldCouldBeNumber {
			fieldCouldBeNumber = isdigit(b) || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-'
		}
	}
	if escapeCount > 0 {
		return csvFieldToStringScalar(field, escapeCount)
	}
	if fieldCouldBeNumber {
		reader := strings.NewReader(field)
		scanner := scanner.NewScanner(reader)
		scalar, err := parseNumber(scanner)
		if err == nil && reader.Len() == 0 {
			return scalar
		}
	}
	scalar := simpleCSVFieldToStringScalar(field)
	if fieldIsAlnum {
		scalar.TypeAndFlags |= token.AlnumMask
	}
	if isHeader {
		scalar.TypeAndFlags |= token.KeyMask
	}
	return scalar
}

func simpleCSVFieldToStringScalar(field string) *token.Scalar {
	var tokenBytes = make([]byte, len(field)+2)
	tokenBytes[0] = '"'
	copy(tokenBytes[1:], []byte(field))
	tokenBytes[len(tokenBytes)-1] = '"'
	return token.NewScalar(token.String, tokenBytes)
}

func csvFieldToStringScalar(field string, escapeCount int) *token.Scalar {
	var tokenBytes = make([]byte, len(field)+escapeCount+2)
	tokenBytes[0] = '"'
	var i = 1
	for _, b := range []byte(field) {
		switch b {
		case '\\', '"':
			tokenBytes[i] = '\\'
			i++
		case '\n':
			tokenBytes[i] = '\\'
			i++
			b = 'n'
		}
		tokenBytes[i] = b
		i++
	}
	tokenBytes[i] = '"'
	return token.NewScalar(token.String, tokenBytes)
}
