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

	"github.com/arnodel/jsonstream/encoding/csv"
	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/encoding/jpv"
	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/internal/jsonpath"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
	"github.com/arnodel/jsonstream/transform"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

var strictMode bool

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
	var jsonIndent int
	var jsonCompact int
	var outputFormat string
	var inputFormat string
	var colorizer *format.Colorizer
	var jpvQuoteKeys bool
	var csvHeader string
	var colorMode string

	if isatty.IsTerminal(os.Stdout.Fd()) {
		colorizer = &defaultColorizer
	}

	// New flags
	flag.IntVar(&jsonIndent, "json-indent", 2, "JSON indentation (use -1 for compact)")
	flag.IntVar(&jsonCompact, "json-compact", 60, "max width for compact JSON arrays/objects")
	flag.BoolVar(&jpvQuoteKeys, "jpv-quote-keys", false, "always quote keys in JPV output")
	flag.StringVar(&colorMode, "color", "auto", "colorize output: auto, always, never")
	flag.StringVar(&outputFormat, "out", "json", "output format: json, jpv")
	flag.StringVar(&inputFormat, "in", "auto", "input format: auto, json, csv, csv-with-header, csvh, jpv")
	flag.StringVar(&csvHeader, "csv-header", "", "comma-separated field names for CSV (only with -in csv)")
	flag.BoolVar(&strictMode, "strict", false, "execute JSONPath query in strict mode")

	// Deprecated flags (kept for backward compatibility)
	flag.IntVar(&jsonIndent, "indent", 2, "DEPRECATED: use -json-indent")
	flag.IntVar(&jsonCompact, "compactwidth", 60, "DEPRECATED: use -json-compact")
	flag.BoolVar(&jpvQuoteKeys, "quotekeys", false, "DEPRECATED: use -jpv-quote-keys")
	flag.BoolFunc("colors", "DEPRECATED: use --color=always", func(s string) error {
		colorMode = "always"
		return nil
	})
	flag.BoolFunc("nocolors", "DEPRECATED: use --color=never", func(s string) error {
		colorMode = "never"
		return nil
	})
	var deprecatedFile string
	flag.StringVar(&deprecatedFile, "file", "", "DEPRECATED: use shell redirection (< file)")

	flag.Parse()

	// Handle color mode
	switch colorMode {
	case "always":
		colorizer = &defaultColorizer
	case "never":
		colorizer = nil
	case "auto":
		// Already set based on isatty check above
	default:
		fatalError("invalid --color value: %q (use auto, always, or never)", colorMode)
	}

	// Warn about deprecated -file flag
	if deprecatedFile != "" {
		fatalError("-file flag removed. Use shell redirection instead: jp [options] [transforms] < %s", deprecatedFile)
	}

	// Set up stdout for handling colors
	var stdout io.Writer = os.Stdout
	if colorizer != nil {
		stdout = colorable.NewColorableStdout()
	}

	// Read from stdin
	var input io.Reader = os.Stdin

	// Choose the input decoder
	if inputFormat == "auto" {
		var start = make([]byte, 40)
		n, err := input.Read(start)
		if err == io.EOF {
			fatalError("unable to guess format of empty file")
		}
		if err != nil {
			fatalError("unable to read input: %s", err)
		}
		start = start[:n]
		inputFormat = guessFormat(start)
		if inputFormat == "" {
			fatalError("unable to guess input format, please specify -in FORMAT")
		}
		input = io.MultiReader(bytes.NewReader(start[:n]), input)
	}

	// Validate CSV options
	if csvHeader != "" && (inputFormat == "csv-with-header" || inputFormat == "csvh") {
		fatalError("-csv-header cannot be used with -in csv-with-header (header row already in file)")
	}

	var decoder token.StreamSource

	switch inputFormat {
	case "json":
		decoder = json.NewDecoder(input)
	case "jpv", "path":
		decoder = jpv.NewDecoder(input)
	case "csv":
		csvDecoder := csv.NewDecoder(input)
		if csvHeader != "" {
			csvDecoder.SetFieldNames(strings.Split(csvHeader, ","))
			csvDecoder.RecordsProduceObjects = true
		}
		decoder = csvDecoder
	case "csv-with-header", "csvh", "csv-header":
		csvDecoder := csv.NewDecoder(input)
		csvDecoder.HasHeader = true
		csvDecoder.RecordsProduceObjects = true
		decoder = csvDecoder
	default:
		fatalError("invalid input format: %q", inputFormat)
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

	printer := &format.DefaultPrinter{
		Writer:     out,
		IndentSize: jsonIndent,
	}

	// If we are writing to a terminal, flush after each line so user gets feedback early.
	if isatty.IsTerminal(os.Stdout.Fd()) {
		printer.Flusher = out
	}

	var encoder token.StreamSink
	switch outputFormat {
	case "json":
		encoder = &json.Encoder{
			Printer:               printer,
			Colorizer:             colorizer,
			CompactWidthLimit:     jsonCompact,
			CompactObjectMaxItems: 2,
		}
	case "jpv", "path":
		{
			jpvEncoder := &jpv.Encoder{Printer: printer, Colorizer: colorizer}
			jpvEncoder.AlwaysQuoteKeys = jpvQuoteKeys
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
		return iterator.AsStreamTransformer(transform.ExplodeArray{}), nil
	}
	if arg == "join" {
		return transform.JoinStream{}, nil
	}
	if arg == "trace" {
		return transform.TraceStream{}, nil
	}
	if strings.HasPrefix(arg, "...") {
		key := strings.TrimPrefix(arg, "...")
		return nil, fmt.Errorf("'%s' syntax removed. Use JSONPath instead: '$..%s'", arg, key)
	}
	if strings.HasPrefix(arg, ".") {
		key := strings.TrimPrefix(arg, ".")
		return nil, fmt.Errorf("'%s' syntax removed. Use JSONPath instead: '$.%s'", arg, key)
	}
	if strings.HasPrefix(arg, "depth=") {
		depth, err := strconv.ParseInt(strings.TrimPrefix(arg, "depth="), 10, 64)
		if err != nil {
			return nil, err
		}
		return &transform.MaxDepthFilter{MaxDepth: int(depth)}, nil
	}
	if strings.HasPrefix(arg, "$") {
		query, err := jsonpath.ParseQueryString(arg)
		if err != nil {
			return nil, err
		}
		return jsonpathtransformer.CompileQuery(query, jsonpathtransformer.WithStrictMode(strictMode))
	}
	return nil, errors.New("invalid transform")
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
	formatGuesser("csv-with-header", `^[a-zA-Z][a-zA-Z_0-9-]*(,[a-zA-Z][a-zA-Z_0-9-]*)+(\n|,?$)`),
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
var defaultColorizer = format.Colorizer{
	ScalarColorCodes: [4][]byte{DimWhite, Yellow, White, Green},
	KeyColorCode:     BrightBlue,
	ResetCode:        Reset,
}
