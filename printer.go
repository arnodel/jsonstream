package jsonstream

import (
	"fmt"
	"io"
)

// The Printer interface can be used to output some structured data.
//
// Indent() starts a new line at an increased indentation level
// Dedent() starts a new line at a decreased indentation level
// NewLine() start a new line at the current indentation level
// PrintBytes() outputs bytes at the current position
//
// The methods do not return an error because for this program it's assumed
// to be an exceptional case that outputting results in an error and the only
// sensible outcome is to stop the program.
// Instead, implementations are expected to panic with a *PrinterError when
// they encounter and error.  A user of the Printer interface can use
//
//	func printingFunction(p Printer) (err error) {
//	    defer CatchPrinterError(&err)
//	    return doSomePrinting(printer)
//	}
//
// to capture such errors.
type Printer interface {
	Indent()
	Dedent()
	NewLine()
	PrintBytes([]byte)
}

// CatchPrinterError can be used to capture panics caused by a Printer because
// of an error encountered while attempting to send output.  See the Printer
// interface documentation for details.
func CatchPrinterError(err *error) {
	if r := recover(); r != nil {
		perr, ok := r.(*PrinterError)
		if ok {
			*err = perr
		} else {
			panic(r)
		}
	}
}

// A PrinterError contains an error that occurred while a Printer implementation
// was sending some output.
type PrinterError struct {
	Err error
}

func (e *PrinterError) Error() string {
	return fmt.Sprintf("printer error: %s", e.Err)
}

func (e *PrinterError) Unwrap() error {
	return e.Err
}

// DefaultPrinter implements a Printer which uses an io.Writer to send output,
// using IndentSize spaces for each indent level.
// If IndentSize is negative, then NewLine() does nothing so all the output
// is on one single line.
// If IndentSize is 0, then there is no indentation but there are still new
// lines.
type DefaultPrinter struct {
	io.Writer
	IndentSize  int
	indentLevel int
}

var _ Printer = &DefaultPrinter{}

// NewLines outputs '\n' followed by a number of spaces corresponding to the
// current indentation level.
func (p *DefaultPrinter) NewLine() {
	if p.IndentSize < 0 {
		return
	}
	_, err := p.Write([]byte{'\n'})
	if err != nil {
		panic(wrapError(err))
	}
	for i := p.IndentSize * p.indentLevel; i > 0; i-- {
		_, err = p.Write([]byte{' '})
		if err != nil {
			panic(wrapError(err))
		}
	}
}

// Indent has the effect of incrementing the indentation level and calls NewLine()
func (p *DefaultPrinter) Indent() {
	p.indentLevel++
	p.NewLine()
}

// Dedent has the effect of decrementing the indentation level and calls NewLine()
func (p *DefaultPrinter) Dedent() {
	p.indentLevel--
	p.NewLine()
}

// PrintBytes sends the gives bytes verbatim to the printer's writer.
func (p *DefaultPrinter) PrintBytes(b []byte) {
	_, err := p.Write(b)
	if err != nil {
		panic(wrapError(err))
	}
}

func wrapError(err error) *PrinterError {
	return &PrinterError{Err: err}
}
