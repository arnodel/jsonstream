package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/arnodel/jsonstream"
)

func main() {
	// Do not handle SIGPIPE, we'll do it ourselves (see error handling at the bottom of main).
	signal.Ignore(syscall.SIGPIPE)

	// Display a stack trace on panic
	defer func() {
		if e := recover(); e != nil {
			fmt.Fprintf(os.Stderr, "%s: %s", e, debug.Stack())
		}
	}()

	// Parse the command line arguments
	var filename string
	var indent int
	var outputFormat string
	var inputFormat string
	flag.StringVar(&filename, "file", "", "json input filename (stdin if omitted)")
	flag.IntVar(&indent, "indent", 2, "indent step for json output (negative means no new lines)")
	flag.StringVar(&outputFormat, "out", "json", "output format")
	flag.StringVar(&inputFormat, "in", "auto", "input format")
	flag.Parse()

	// Open input file
	var input *os.File
	if filename != "" {
		var err error
		input, err = os.Open(filename)
		if err != nil {
			fatalError("error opening %q: %s", filename, err)
		}
	} else {
		input = os.Stdin
	}
	bufInput := bufio.NewReader(input)

	// Choose the input decoder
	if inputFormat == "auto" {
		start, err := bufInput.Peek(10)
		if err != nil {
			fatalError("unable to read input: %s", err)
		}
		inputFormat = guessFormat(start)
	}

	var decoder jsonstream.StreamSource

	switch inputFormat {
	case "json":
		decoder = jsonstream.NewJSONDecoder(bufInput)
	case "jpv", "path":
		decoder = jsonstream.NewJPVDecoder(bufInput)
	default:
		fmt.Fprintf(os.Stderr, "invalid input format: %q", outputFormat)
		os.Exit(1)
	}

	// Start parsing the input file
	stream := jsonstream.StartStream(
		decoder,
		func(err error) {
			fmt.Fprintf(os.Stderr, "error while parsing: %s", err)
		},
	)

	// Parse transforms and apply them sequentially
	for _, arg := range flag.Args() {
		transformer, err := parseTransformer(arg)
		if err != nil {
			fatalError("error: %s", err)
		}
		stream = jsonstream.TransformStream(stream, transformer)
	}

	// Write the output stream to stdout
	stdout := bufio.NewWriter(os.Stdout)
	defer stdout.Flush()

	printer := &jsonstream.DefaultPrinter{
		Writer:     stdout,
		IndentSize: indent,
	}

	var encoder jsonstream.StreamSink
	switch outputFormat {
	case "json":
		encoder = &jsonstream.JSONEncoder{Printer: printer}
	case "jpv", "path":
		encoder = &jsonstream.JPVEncoder{Printer: printer}
	default:
		fatalError("invalid output format: %q", outputFormat)
	}
	err := jsonstream.ConsumeStream(stream, encoder)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			// stdout is a pipe and something closed it (e.g. 'head' or 'less').
			// In this case we don't want to complain.
			return
		}
		fatalError("error: %s", err)
	}
}

func parseTransformer(arg string) (jsonstream.StreamTransformer, error) {
	if arg == "split" {
		return jsonstream.AsStreamTransformer(jsonstream.ExplodeArray{}), nil
	}
	if arg == "join" {
		return jsonstream.JoinStream{}, nil
	}
	if arg == "trace" {
		return jsonstream.TraceStream{}, nil
	}
	if strings.HasPrefix(arg, "...") {
		return jsonstream.AsStreamTransformer(&jsonstream.DeepKeyExtractor{Key: strings.TrimPrefix(arg, "...")}), nil
	}
	if strings.HasPrefix(arg, ".") {
		return jsonstream.AsStreamTransformer(&jsonstream.KeyExtractor{Key: strings.TrimPrefix(arg, ".")}), nil
	}
	if strings.HasPrefix(arg, "depth=") {
		depth, err := strconv.ParseInt(strings.TrimPrefix(arg, "depth="), 10, 64)
		if err != nil {
			return nil, err
		}
		return &jsonstream.MaxDepthFilter{MaxDepth: int(depth)}, nil
	}
	return nil, errors.New("invalid filter")
}

func guessFormat(start []byte) string {
	if len(start) == 0 {
		return "json"
	}
	switch start[0] {
	case '$':
		return "jpv"
	default:
		return "json"
	}
}

func fatalError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}
