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
	flag.StringVar(&filename, "file", "", "json input filename (stdin if omitted)")
	flag.IntVar(&indent, "indent", 2, "indent step for json output (negative means no new lines)")
	flag.Parse()

	// Open input file
	var input *os.File
	if filename != "" {
		var err error
		input, err = os.Open(filename)
		if err != nil {
			panic(err)
		}
	} else {
		input = os.Stdin
	}

	// Start parsing the input file
	stream := jsonstream.StartStream(
		jsonstream.NewJSONReader(input),
		func(err error) {
			fmt.Fprintf(os.Stderr, "error while parsing: %s", err)
		},
	)

	// Parse transforms and apply them sequentially
	for _, arg := range flag.Args() {
		filter, err := parseTransform(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s", err)
			return
		}
		stream = jsonstream.TransformStream(stream, filter)
	}

	// Write the output stream to stdout
	stdout := bufio.NewWriter(os.Stdout)
	defer stdout.Flush()

	printer := &jsonstream.DefaultPrinter{
		Writer:     stdout,
		IndentSize: indent,
	}

	err := jsonstream.ConsumeStream(
		stream,
		&jsonstream.JSONEncoder{Printer: printer},
	)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			// stdout is a pipe and something closed it (e.g. 'head' or 'less').
			// In this case we don't want to complain.
			return
		}
		fmt.Fprintf(os.Stderr, "error: %s", err)
	}
}

func parseTransform(arg string) (jsonstream.StreamTransformer, error) {
	if arg == "split" {
		return jsonstream.AsStreamTransformer(jsonstream.ExplodeArray{}), nil
	}
	if arg == "join" {
		return jsonstream.JoinStream{}, nil
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
