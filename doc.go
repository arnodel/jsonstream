package jsonstream

// Package jsonstream implements routines for processing JSON input in a stream.
//
// The package is organized into several sub-packages:
//
// - encoding/json: JSON decoder and encoder
// - encoding/csv: CSV decoder
// - encoding/jpv: JPV (JSON Path-Value) decoder and encoder
// - transform: Built-in stream transformers
// - transform/jsonpath: JSONPath query execution
// - token: Core token-based streaming infrastructure
// - iterator: Value-based iteration over token streams
//
// These can be combined to form a JSON processing pipeline:
//
//    decode JSON -> transform_1 -> ... -> transform_n -> encode JSON
//
// Each stage in the pipeline is a streaming operation, so the whole pipeline
// can start producing output straight away. This provides several advantages:
//
// - Extract relevant data from JSON input without memory usage increasing
//   with the size of the input (constant memory usage)
// - When piping output through tools like 'less' or 'head', output is
//   available immediately without waiting for the entire file to be processed
// - Process arbitrarily large or infinite JSON streams
//
// This package was designed for the jp CLI utility that processes JSON input.
// There is no facility for marshaling or unmarshaling Go structures, unlike
// the standard library encoding/json package.
//
// The CLI utility is in the directory cmd/jp. You can install it with:
//
//  go install github.com/arnodel/jsonstream/cmd/jp
//
