package jsonstream

// Package jsonstream implements routines for processing JSON input in a stream.
//
// - reading JSON input from an [io.Reader] into a JSON stream: [JSONDecoder]
// - transforming a JSON stream into another JSON stream: [StreamTransformer]
// - writing a JSON stream to an [io.Writer]: [JSONEncoder]
//
// Those can be combined to form a JSON processing pipeline
//
//    decode JSON -> transform_1 -> ... -> transform_n -> encode JSON
//
// Each of the items in the pipeline is a streaming operation so the whole pipeline
// can start producing output straight away.  This can be an advantage for different reasons,
// e.g.
//
// - it is possible to extract relevant data from JSON input without memory usage increasing
//   with the size of the input.
// - when piping the output of a jsonstream based program through to e.g. 'less' or 'head',
//   the next program can be fed input very early, before the whole file is processed.
//
// This package has been made with the goal writing a CLI utility that processes JSON input,
// so there is no facility for marshaling or unmarshaling as in the standard library
// [encoding/json] package.
//
// The CLI utility is in the directory cmd/jp.  You can install it with
//
//  go install github.com/arnodel/jsonstream/cmd/jp
