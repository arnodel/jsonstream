// Package format provides the output side of the jsonstream pipeline: the
// Printer interface that encoders write through, DefaultPrinter which
// implements it over an io.Writer with indentation, and Colorizer for ANSI
// colour output.
//
// Most users do not need this package directly: json.NewEncoder and
// jpv.NewEncoder set up a DefaultPrinter with sensible defaults.  Use it to
// control indentation, to flush after every line (DefaultPrinter.Flusher, good
// when writing to a terminal), to add colours, or to implement Printer for a
// different kind of output.
package format
