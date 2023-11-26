package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/arnodel/jsonstream"
	"github.com/arnodel/jsonstream/internal/jsonpath"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/jsonpathtransformer"
	"github.com/arnodel/jsonstream/token"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
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
	var colorizer *jsonstream.Colorizer
	var quoteKeys bool

	if isatty.IsTerminal(os.Stdout.Fd()) {
		colorizer = &defaultColorizer
	}

	flag.BoolFunc("colors", "force using colors", func(s string) error {
		colorizer = &defaultColorizer
		return nil
	})
	flag.BoolFunc("nocolors", "disable colors", func(s string) error {
		colorizer = nil
		return nil
	})

	flag.StringVar(&filename, "file", "", "json input filename (stdin if omitted)")
	flag.IntVar(&indent, "indent", 2, "indent step for json output (negative means no new lines)")
	flag.StringVar(&outputFormat, "out", "json", "output format")
	flag.StringVar(&inputFormat, "in", "auto", "input format")
	flag.BoolVar(&quoteKeys, "quotekeys", false, "always use quoted keys in JSON Path output")
	flag.Parse()

	// Set up stdout for handling colors

	var stdout io.Writer = os.Stdout
	if colorizer != nil {
		stdout = colorable.NewColorableStdout()
	}

	// Open input file
	var input io.Reader
	if filename != "" {
		var err error
		input, err = os.Open(filename)
		if err != nil {
			fatalError("error opening %q: %s", filename, err)
		}
	} else {
		input = os.Stdin
	}

	// Choose the input decoder
	if inputFormat == "auto" {
		var start = make([]byte, 40)
		_, err := input.Read(start)
		if err == io.EOF {
			fatalError("unable to guess format of empty file")
		}
		if err != nil {
			fatalError("unable to read input: %s", err)
		}
		inputFormat = guessFormat(start)
		if inputFormat == "" {
			fatalError("unable to guess input format, please specify -in FORMAT")
		}
		input = io.MultiReader(bytes.NewReader(start), input)
	}

	var decoder token.StreamSource

	switch inputFormat {
	case "json":
		decoder = jsonstream.NewJSONDecoder(input)
	case "jpv", "path":
		decoder = jsonstream.NewJPVDecoder(input)
	case "csv":
		decoder = jsonstream.NewCSVDecoder(input)
	case "csv-header", "csvh":
		csvDecoder := jsonstream.NewCSVDecoder(input)
		csvDecoder.HasHeader = true
		csvDecoder.RecordsProduceObjects = true
		decoder = csvDecoder
	default:
		fatalError("invalid input format: %q", outputFormat)
	}

	// Start parsing the input file
	stream := token.StartStream(
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
		stream = token.TransformStream(stream, transformer)
	}

	// Write the output stream to stdout
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	printer := &jsonstream.DefaultPrinter{
		Writer:     out,
		IndentSize: indent,
	}

	// If we are writing to a terminal, flush after each line so user gets feedback early.
	if isatty.IsTerminal(os.Stdout.Fd()) {
		printer.Flusher = out
	}

	var encoder token.StreamSink
	switch outputFormat {
	case "json":
		encoder = &jsonstream.JSONEncoder{Printer: printer, Colorizer: colorizer}
	case "jpv", "path":
		{
			jpvEncoder := &jsonstream.JPVEncoder{Printer: printer, Colorizer: colorizer}
			jpvEncoder.AlwaysQuoteKeys = quoteKeys
			encoder = jpvEncoder
		}
	default:
		fatalError("invalid output format: %q", outputFormat)
	}

	err := token.ConsumeStream(stream, encoder)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			// stdout is a pipe and something closed it (e.g. 'head' or 'less').
			// In this case we don't want to complain.
			return
		}
		fatalError("error: %s", err)
	}
}

func parseTransformer(arg string) (token.StreamTransformer, error) {
	if arg == "split" {
		return iterator.AsStreamTransformer(jsonstream.ExplodeArray{}), nil
	}
	if arg == "join" {
		return jsonstream.JoinStream{}, nil
	}
	if arg == "trace" {
		return jsonstream.TraceStream{}, nil
	}
	if strings.HasPrefix(arg, "...") {
		return iterator.AsStreamTransformer(&jsonstream.DeepKeyExtractor{Key: strings.TrimPrefix(arg, "...")}), nil
	}
	if strings.HasPrefix(arg, ".") {
		return iterator.AsStreamTransformer(&jsonstream.KeyExtractor{Key: strings.TrimPrefix(arg, ".")}), nil
	}
	if strings.HasPrefix(arg, "depth=") {
		depth, err := strconv.ParseInt(strings.TrimPrefix(arg, "depth="), 10, 64)
		if err != nil {
			return nil, err
		}
		return &jsonstream.MaxDepthFilter{MaxDepth: int(depth)}, nil
	}
	if strings.HasPrefix(arg, "$") {
		query, err := jsonpath.ParseQueryString(arg)
		if err != nil {
			return nil, err
		}
		return jsonpathtransformer.CompileQuery(query), nil
	}
	return nil, errors.New("invalid filter")
}

type FormatGuesser struct {
	pattern *regexp.Regexp
	format  string
}

func formatGuesser(format string, pattern string) FormatGuesser {
	return FormatGuesser{
		pattern: regexp.MustCompile(pattern),
		format:  format,
	}
}

var formatGuessers = []FormatGuesser{
	formatGuesser("jpv", `^$`),
	formatGuesser("json", `^[{[]`),
	formatGuesser("csv-header", `^[a-zA-Z][a-zA-Z_0-9-]*(,[a-zA-Z][a-zA-Z_0-9-]*)+(\n|,?$)`),
	formatGuesser("csv", `^([^,"\n]*|("[^"]*"))(,[^,"\n]*|,("[^"]*"))+(\n|,?$)`),
}

func guessFormat(start []byte) string {
	for _, guesser := range formatGuessers {
		if guesser.pattern.Match(start) {
			return guesser.format
		}
	}
	return ""
}

func fatalError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

// Some color ANSI codes
var (
	Reset = []byte("\033[0m")

	Black   = []byte("\033[30m")
	Red     = []byte("\033[31m")
	Green   = []byte("\033[32m")
	Yellow  = []byte("\033[33m")
	Blue    = []byte("\033[34m")
	Magenta = []byte("\033[35m")
	Cyan    = []byte("\033[36m")
	White   = []byte("\033[37m")

	DimBlack   = []byte("\033[30;2m")
	DimRed     = []byte("\033[31;2m")
	DimGreen   = []byte("\033[32;2m")
	DimYellow  = []byte("\033[33;2m")
	DimBlue    = []byte("\033[34;2m")
	DimMagenta = []byte("\033[35;2m")
	DimCyan    = []byte("\033[36;2m")
	DimWhite   = []byte("\033[37;2m")

	BrightBlack   = []byte("\033[30;1m")
	BrightRed     = []byte("\033[31;1m")
	BrightGreen   = []byte("\033[32;1m")
	BrightYellow  = []byte("\033[33;1m")
	BrightBlue    = []byte("\033[34;1m")
	BrightMagenta = []byte("\033[35;1m")
	BrightCyan    = []byte("\033[36;1m")
	BrightWhite   = []byte("\033[37;1m")
)

// The colors I chose :)
var defaultColorizer = jsonstream.Colorizer{
	ScalarColorCodes: [4][]byte{DimWhite, Yellow, White, Green},
	KeyColorCode:     BrightBlue,
	ResetCode:        Reset,
}
