package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// A Decoder reads CSV input and streams it into a JSON stream.
type Decoder struct {
	reader                *csv.Reader
	HasHeader             bool // When true, treat the first record as a header
	RecordsProduceObjects bool // When false, produce an array for each record, else an object
	fieldNames            []*token.Scalar
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given input.
func NewDecoder(in io.Reader) *Decoder {
	return &Decoder{reader: csv.NewReader(in)}
}

// Produce reads a stream of CSV records, until it runs out of input or
// encounters invalid CSV, in which case it will return an error
func (d *Decoder) Produce(out chan<- token.Token) error {
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

// SetFieldNames sets the field names for records.  Should be called before Produce.
func (d *Decoder) SetFieldNames(record []string) {
	for _, field := range record {
		d.fieldNames = append(d.fieldNames, fieldToScalar(field, true))
	}
}

func (d *Decoder) produceRecord(record []string, out chan<- token.Token) {
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

func (d *Decoder) getFieldName(i int) *token.Scalar {
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
			fieldIsAlnum = scanner.IsAlpha(b)
		} else {
			fieldIsAlnum = fieldIsAlnum && scanner.IsAlnum(b)
		}
		if fieldCouldBeNumber {
			fieldCouldBeNumber = scanner.IsDigit(b) || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-'
		}
	}
	if escapeCount > 0 {
		return csvFieldToStringScalar(field, escapeCount)
	}
	if fieldCouldBeNumber {
		reader := strings.NewReader(field)
		scanr := scanner.NewScanner(reader)
		scalar, err := json.ParseNumber(scanr)
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

var (
	trueInstance  = token.NewScalar(token.Boolean, []byte("true"))
	falseInstance = token.NewScalar(token.Boolean, []byte("false"))
	nullInstance  = token.NewScalar(token.Null, []byte("null"))
)
