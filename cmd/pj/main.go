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
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arnodel/jsonstream/encoding/csv"
	"github.com/arnodel/jsonstream/encoding/jpv"
	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/format"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
	"github.com/arnodel/jsonstream/transform"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

var strictMode bool

// idleFlushTimeout is how long output may sit in the stdout buffer while
// waiting for more input.  When the input stalls (e.g. 'tail -f app.log | pj |
// less'), results already produced are flushed after this delay instead of
// waiting in the buffer until more input arrives or the buffer fills up.
const idleFlushTimeout = 100 * time.Millisecond

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
	var jsonCompact bool
	var jsonCompactWidth int
	var outputFormat string
	var inputFormat string
	var colorizer *format.Colorizer
	var jpvQuoteKeys bool
	var csvHeader string
	var colorMode string
	var helpInput bool
	var helpOutput bool
	var helpTransforms bool
	var helpCookbook bool

	if isatty.IsTerminal(os.Stdout.Fd()) {
		colorizer = &defaultColorizer
	}

	// Custom usage function
	flag.Usage = printUsage

	// Help flags
	flag.BoolVar(&helpInput, "help-input", false, "show detailed help for input formats")
	flag.BoolVar(&helpOutput, "help-output", false, "show detailed help for output formats")
	flag.BoolVar(&helpTransforms, "help-transforms", false, "show detailed help for transforms")
	flag.BoolVar(&helpCookbook, "help-cookbook", false, "show cookbook with common usage patterns")

	// New flags
	flag.IntVar(&jsonIndent, "json-indent", 2, "JSON indentation level (only used when -json-compact is false)")
	flag.BoolVar(&jsonCompact, "json-compact", false, "output JSON on a single line (JSON Lines format)")
	flag.BoolVar(&jsonCompact, "json-lines", false, "output in JSON Lines format (alias for -json-compact)")
	flag.IntVar(&jsonCompactWidth, "json-compact-width", 60, "max width for compact JSON arrays/objects")
	flag.BoolVar(&jpvQuoteKeys, "jpv-quote-keys", false, "always quote keys in JPV output")
	flag.StringVar(&colorMode, "color", "auto", "colorize output: auto, always, never")
	flag.StringVar(&outputFormat, "out", "json", "output format: json, jpv")
	flag.StringVar(&inputFormat, "in", "auto", "input format: auto, json, csv, csv-with-header, csvh, jpv")
	flag.StringVar(&csvHeader, "csv-header", "", "comma-separated field names for CSV (only with -in csv)")
	flag.BoolVar(&strictMode, "jsonpath-strict", false, "execute JSONPath queries in strict mode")

	// Deprecated flags (kept for backward compatibility)
	flag.IntVar(&jsonIndent, "indent", 2, "DEPRECATED: use -json-indent")
	flag.IntVar(&jsonCompactWidth, "compactwidth", 60, "DEPRECATED: use -json-compact-width")
	flag.BoolVar(&jpvQuoteKeys, "quotekeys", false, "DEPRECATED: use -jpv-quote-keys")
	flag.BoolFunc("colors", "DEPRECATED: use -color=always", func(s string) error {
		colorMode = "always"
		return nil
	})
	flag.BoolFunc("nocolors", "DEPRECATED: use -color=never", func(s string) error {
		colorMode = "never"
		return nil
	})
	var inputFile string
	flag.StringVar(&inputFile, "file", "", "read input from FILE (same as passing FILE as an argument)")

	flag.Parse()

	// Handle help flags
	if helpInput {
		printInputHelp()
		return
	}
	if helpOutput {
		printOutputHelp()
		return
	}
	if helpTransforms {
		printTransformsHelp()
		return
	}
	if helpCookbook {
		printCookbookHelp()
		return
	}

	// Handle color mode
	switch colorMode {
	case "always":
		colorizer = &defaultColorizer
	case "never":
		colorizer = nil
	case "auto":
		// Already set based on isatty check above
	default:
		fatalError("invalid -color value: %q (use auto, always, or never)", colorMode)
	}

	// Set up stdout for handling colors
	var stdout io.Writer = os.Stdout
	if colorizer != nil {
		stdout = colorable.NewColorableStdout()
	}

	// Positional arguments are transforms, except those naming input files.
	var transformArgs, inputFiles []string
	for _, arg := range flag.Args() {
		if isTransformArg(arg) {
			transformArgs = append(transformArgs, arg)
		} else {
			inputFiles = append(inputFiles, arg)
		}
	}
	if inputFile != "" {
		inputFiles = append([]string{inputFile}, inputFiles...)
	}

	// Read from the input files, or from stdin if there are none.  The input
	// mode (single value or stream of values) is inferred from the file names
	// when possible; -in json or -in json-stream override it below.
	var input io.Reader = os.Stdin
	inputMode := jsonpathtransformer.AutoDetectInput
	if len(inputFiles) > 0 {
		readers := make([]io.Reader, 0, len(inputFiles))
		for _, name := range inputFiles {
			f, err := os.Open(name)
			if err != nil {
				fatalError("%s", err)
			}
			defer f.Close()
			readers = append(readers, f)
		}
		input = io.MultiReader(readers...)
		inputMode = inputModeFromFiles(inputFiles)
	}

	// Choose the input decoder
	explicitFormat := inputFormat != "auto"
	if !explicitFormat {
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
			// Nothing matched (e.g. a top-level scalar such as 42 or "hello"),
			// fall back to JSON and let the decoder report any syntax error.
			inputFormat = "json"
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
		if explicitFormat {
			inputMode = jsonpathtransformer.SingleValueInput
		}
	case "json-stream", "jsonl", "ndjson":
		decoder = json.NewDecoder(input)
		if explicitFormat {
			inputMode = jsonpathtransformer.ValueStreamInput
		}
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
		fatalError("invalid input format: %q (use json, json-stream, jpv, csv or csv-with-header)", inputFormat)
	}

	// Start parsing the input file
	stream := token.StartStream(
		decoder,
		func(err error) {
			fmt.Fprintf(os.Stderr, "error while parsing: %s", err)
		},
	)

	// Parse transforms and apply them sequentially.  Each transform is told
	// whether its input is a single value or a stream, and says the same about
	// its output for the next one.
	mode := inputMode
	for _, arg := range transformArgs {
		transformer, outMode, err := parseTransformer(arg, mode)
		if err != nil {
			fatalError("error: %s", err)
		}
		stream = token.TransformStream(stream, transformer)
		mode = outMode
	}

	// Write the output stream to stdout
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	// Set up printer with appropriate indentation
	indentSize := jsonIndent
	if jsonCompact {
		indentSize = -1
	}

	printer := &format.DefaultPrinter{
		Writer:     out,
		IndentSize: indentSize,
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
			CompactWidthLimit:     jsonCompactWidth,
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

	// Feed the encoder, flushing the output when the input stalls.
	reader := stream.Reader()
	reader.IdleTimeout = idleFlushTimeout
	reader.OnIdle = func() {
		if err := out.Flush(); err != nil {
			// Reported the same way as any other output error.
			panic(&format.PrinterError{Err: err})
		}
	}
	err := encoder.Consume(reader)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			// stdout is a pipe and something closed it (e.g. 'head' or 'less').
			// In this case we don't want to complain.
			return
		}
		fatalError("error: %s", err)
	}
}

// isTransformArg tells whether a positional argument is a transform (as
// opposed to an input file name).
func isTransformArg(arg string) bool {
	switch {
	case arg == "split", arg == "join", arg == "trace":
		return true
	case strings.HasPrefix(arg, "depth="), strings.HasPrefix(arg, "$"), strings.HasPrefix(arg, "."):
		return true
	}
	return false
}

// inputModeFromFiles infers whether the input is a single JSON value or a
// stream of values from the names of the input files.
func inputModeFromFiles(files []string) jsonpathtransformer.InputMode {
	if len(files) > 1 {
		// Several files, so several values.
		return jsonpathtransformer.ValueStreamInput
	}
	switch strings.ToLower(filepath.Ext(files[0])) {
	case ".json":
		return jsonpathtransformer.SingleValueInput
	case ".jsonl", ".ndjson":
		return jsonpathtransformer.ValueStreamInput
	default:
		return jsonpathtransformer.AutoDetectInput
	}
}

// parseTransformer turns a command line argument into a transformer.  It is
// given the input mode of the transformer (whether it will receive a single
// value or a stream of values) and returns the input mode of the next one.
func parseTransformer(arg string, mode jsonpathtransformer.InputMode) (token.StreamTransformer, jsonpathtransformer.InputMode, error) {
	if arg == "split" {
		return iterator.AsStreamTransformer(transform.ExplodeArray{}), jsonpathtransformer.ValueStreamInput, nil
	}
	if arg == "join" {
		// join always outputs exactly one value, so a following query can stop
		// as soon as it has its results.
		return transform.JoinStream{}, jsonpathtransformer.SingleValueInput, nil
	}
	if arg == "trace" {
		return transform.TraceStream{}, mode, nil
	}
	if strings.HasPrefix(arg, "...") {
		key := strings.TrimPrefix(arg, "...")
		return nil, mode, fmt.Errorf("'%s' syntax removed. Use JSONPath instead: '$..%s'", arg, key)
	}
	if strings.HasPrefix(arg, ".") {
		key := strings.TrimPrefix(arg, ".")
		return nil, mode, fmt.Errorf("'%s' syntax removed. Use JSONPath instead: '$.%s'", arg, key)
	}
	if strings.HasPrefix(arg, "depth=") {
		depth, err := strconv.ParseInt(strings.TrimPrefix(arg, "depth="), 10, 64)
		if err != nil {
			return nil, mode, err
		}
		return &transform.MaxDepthFilter{MaxDepth: int(depth)}, mode, nil
	}
	if strings.HasPrefix(arg, "$") {
		runner, err := jsonpathtransformer.CompileQueryString(arg,
			jsonpathtransformer.WithStrictMode(strictMode),
			jsonpathtransformer.WithInputMode(mode),
		)
		if err != nil {
			return nil, mode, err
		}
		// A singular query on a single value gives at most one value.
		outMode := jsonpathtransformer.ValueStreamInput
		if runner.IsSingular() && mode == jsonpathtransformer.SingleValueInput {
			outMode = jsonpathtransformer.SingleValueInput
		}
		return runner, outMode, nil
	}
	return nil, mode, errors.New("invalid transform")
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
	formatGuesser("jpv", `^\$`),
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

func printUsage() {
	fmt.Fprint(os.Stderr, `pj - JSON stream processor

USAGE:
  pj [options] [transforms...] [files...]

DESCRIPTION:
  pj processes JSON input in a streaming manner, allowing you to transform,
  filter, and format JSON data with constant memory usage.

  Input is read from the files given as arguments, or from stdin:
    pj file.json
    pj '$.items[*]' file.json
    cat file.json | pj

HELP OPTIONS:
  -help-input       Show detailed help for input formats
  -help-output      Show detailed help for output formats and options
  -help-transforms  Show detailed help for transforms with examples
  -help-cookbook    Show cookbook with Unix-like usage patterns (head/tail/grep)

INPUT/OUTPUT:
  -in FORMAT        Input format (default: auto)
                    Formats: json (a single value), json-stream (or jsonl),
                    jpv, csv, csv-with-header (or csvh)
  -out FORMAT       Output format (default: json)
                    Formats: json, jpv
  -csv-header NAMES Comma-separated field names for CSV input
                    Only valid with '-in csv'

JSON OUTPUT OPTIONS:
  -json-compact         Output in JSON Lines format (one value per line)
  -json-lines           Alias for -json-compact
  -json-indent N        Indentation level (default: 2, only used when not compact)
  -json-compact-width N Max width for inline arrays/objects (default: 60)

JPV OUTPUT OPTIONS:
  -jpv-quote-keys   Always quote keys in JPV output

COLOR OPTIONS:
  -color MODE       Control color output (default: auto)
                    Modes: auto, always, never

TRANSFORMS:
  Transforms are applied sequentially to the input stream.

  '$...'            JSONPath query (use single quotes!)
                    Examples: '$.items[0]' '$..name' '$.users[?@.age > 18]'
  split             Split array into stream of values
  join              Join stream of values into array
  depth=N           Truncate output at depth N
  trace             Log stream to stderr (for debugging)

JSONPATH QUERIES:
  -jsonpath-strict  Execute JSONPath queries in strict mode (RFC-compliant ordering,
                    less streaming-friendly for descendant queries like $..)

  A query stops reading as soon as it cannot produce more results if the
  input is a single JSON value ('-in json', or a .json file argument), and
  skips to the next value if it is a stream ('-in json-stream', a .jsonl
  file). Otherwise pj works it out from the input: see 'pj -help-input'.

EXAMPLES:
  # Pretty-print JSON
  cat data.json | pj

  # Extract specific field from array items
  cat users.json | pj '$.users[*].name'

  # Convert JSON array to JSON Lines (one item per line)
  cat array.json | pj -json-lines split

  # Filter and transform
  cat data.json | pj '$.items[?@.price < 100]' split

  # CSV to JSON Lines
  cat data.csv | pj -in csv-with-header -json-lines

For Unix-like usage patterns (head/tail/grep), see: pj -help-cookbook
For more information, visit: https://github.com/arnodel/jsonstream
`)
}

func printInputHelp() {
	fmt.Fprint(os.Stderr, `pj - Input Format Help

INPUT FORMAT SELECTION:
  Use the '-in FORMAT' flag to specify the input format.
  Default is 'auto', which attempts to detect the format automatically.

AVAILABLE FORMATS:

  json
    A single JSON value, e.g. {"name": "Alice"} or a whole document.

    Knowing that there is only one value lets a JSONPath query stop reading
    as soon as it cannot produce more results. For example with '-in json',
    '$.items[:10]' returns right after the 10th item, however big the
    document, and even if it never ends.

  json-stream (or jsonl, ndjson)
    A sequence of JSON values separated by whitespace. JSON Lines (one value
    per line) is the usual shape, but values may span several lines, as in
    pj's own pretty-printed output. Each value goes separately through the
    transform pipeline:
      {"name": "Alice", "age": 30}
      {"name": "Bob", "age": 25}

    A query always reads to the end of each value, since more may follow.
    Use -json-lines or -json-compact to output in JSON Lines format.

  jpv (or path)
    JSON Path-Value format. Each line specifies a JSONPath and its value.
    Similar to GRON format but uses JSONPath syntax.

    Example:
      $["name"] = "Alice"
      $["items"][0] = "apple"
      $["items"][1] = "orange"

  csv
    Comma-Separated Values format. Each record becomes a JSON array.
    Use '-csv-header' to provide field names and convert to objects.

    Example without -csv-header:
      Input:  John,Doe,30
              Jane,Smith,25
      Output: ["John", "Doe", 30]
              ["Jane", "Smith", 25]

    Example with -csv-header name,surname,age:
      Input:  John,Doe,30
              Jane,Smith,25
      Output: {"name": "John", "surname": "Doe", "age": 30}
              {"name": "Jane", "surname": "Smith", "age": 25}

  csv-with-header (or csvh)
    CSV format where the first row is treated as a header.
    Each subsequent record becomes a JSON object.

    Example:
      Input:  name,surname,age
              John,Doe,30
              Jane,Smith,25
      Output: {"name": "John", "surname": "Doe", "age": 30}
              {"name": "Jane", "surname": "Smith", "age": 25}

  auto (default)
    Detects the format from the first few bytes of input, falling back to
    JSON. For JSON input, decides between json and json-stream as follows:
    - A single file argument named *.json is a single value; *.jsonl or
      *.ndjson is a stream; several files are a stream.
    - Otherwise (stdin, or another extension), the first time a query is
      done with a value before its end, pj skips ahead up to about 1MB. If
      the value ends by then, the input is a stream; if not, it is one big
      value and pj stops reading.
    Values over 1MB in a stream are therefore mistaken for a single value;
    pass '-in json-stream' in that case.

INPUT FILES:
  Files can be given as arguments, in any position relative to transforms:
    pj '$.items[*]' data.json
    pj a.json b.json
  Several files are read one after the other as a stream of values.

NOTES:
  - Empty fields in CSV are converted to null
  - CSV values 'true' and 'false' are converted to booleans
  - CSV numeric values are parsed as JSON numbers
  - Files can also be read with shell redirection: pj -in csv < data.csv
`)
}

func printOutputHelp() {
	fmt.Fprint(os.Stderr, `pj - Output Format Help

OUTPUT FORMAT SELECTION:
  Use the '-out FORMAT' flag to specify the output format.
  Default is 'json'.

AVAILABLE FORMATS:

  json (default)
    Standard JSON format with pretty-printing.

    Default output (with -json-indent 2):
      {
        "name": "Alice",
        "scores": [95, 87, 92],
        "address": {
          "city": "Boston",
          "zip": "02101"
        }
      }

    JSON-SPECIFIC OPTIONS:

    -json-compact (or -json-lines)
      Output in JSON Lines format - each value on a single line.
      This is useful for streaming output or processing with line-based tools.
      Example output:
        {"name": "Alice", "scores": [95, 87, 92]}
        {"name": "Bob", "scores": [88, 91, 95]}

      Combine with 'split' to convert JSON arrays to JSON Lines:
        cat array.json | pj -json-lines split

    -json-indent N
      Set indentation level (default: 2, only applies when not compact).
      With -json-indent 4:
        {
            "name": "Alice",
            "scores": [95, 87, 92]
        }

    -json-compact-width N
      Max width for inline arrays/objects (default: 60).
      Small arrays and objects that fit are displayed inline for readability.
      With default settings, [95, 87, 92] stays inline.
      With -json-compact-width 10, it splits across lines.

  jpv (or path)
    JSON Path-Value format. Each path-value pair on its own line.
    Useful for grepping and filtering specific parts of JSON.

    Default output:
      $.name = "Alice"
      $.scores[0] = 95
      $.scores[1] = 87
      $.scores[2] = 92
      $.address.city = "Boston"
      $.address.zip = "02101"

    JPV-SPECIFIC OPTIONS:

    -jpv-quote-keys
      Always quote keys in brackets, even if alphanumeric.
      With -jpv-quote-keys:
        $["name"] = "Alice"
        $["scores"][0] = 95
        $["address"]["city"] = "Boston"

    Workflow example:
      # Convert to JPV, filter with grep, convert back to JSON
      cat data.json | pj -out jpv | grep city | pj -in jpv

COLOR OPTIONS:
  -color MODE        Control color output (default: auto)

                     auto    - Use colors when outputting to a terminal
                     always  - Always use colors
                     never   - Never use colors

  Colors are applied to:
    - Object keys (bright blue)
    - String values (yellow)
    - Numbers (white)
    - Booleans and null (green)

EXAMPLES:
  # Convert JSON array to JSON Lines
  cat array.json | pj -json-lines split

  # JSON Lines output (compact)
  cat data.json | pj -json-lines

  # No colors even in terminal
  cat data.json | pj -color never

  # Convert JSON to JPV and filter
  cat users.json | pj -out jpv | grep email
`)
}

func printTransformsHelp() {
	fmt.Fprint(os.Stderr, `pj - Transform Help

TRANSFORMS:
  Transforms are applied sequentially to process the JSON stream.
  Specify transforms as positional arguments after flags.

AVAILABLE TRANSFORMS:

  JSONPath Queries: '$...'
    Execute a JSONPath query on the input. IMPORTANT: Use single quotes
    to prevent shell interpretation of special characters like $ and *.

    The full IETF JSONPath spec (RFC 9535) is implemented.

    Examples:
      '$.name'                    - Get the 'name' field
      '$.items[0]'                - Get first item
      '$.items[-1]'               - Get last item
      '$.items[2:5]'              - Slice: items 2, 3, 4
      '$.items[*]'                - All items in array
      '$..name'                   - All 'name' fields at any depth
      '$.items[?@.price < 100]'   - Filter: items where price < 100
      '$.items[?@.name =~ /^A/]'  - Filter: names starting with A
      '$[*].length'               - Get 'length' from all top-level objects

    Use the -jsonpath-strict flag for strict mode (RFC-compliant ordering).

  split
    Splits an array into a stream of its individual values.
    Non-array values pass through unchanged.

    Example:
      Input:  [1, 2, 3]
      Output: 1
              2
              3

  join
    Joins a stream of values into a single array. Since the result is one
    value, a following query can stop reading as soon as it has its
    results: 'pj join '$[:10]'' is 'head' for a stream of values.

    Example:
      Input:  1
              2
              3
      Output: [1, 2, 3]

  depth=N
    Truncates output at the specified depth level.
    Collections deeper than N are replaced with '...'.

    Example with depth=1:
      Input:  {"a": 1, "b": {"c": 2, "d": 3}}
      Output: {"a": 1, "b": {...}}

  trace
    Logs all stream tokens to stderr. Useful for debugging transforms.
    Consumes the stream without producing output.

COMBINING TRANSFORMS:
  Transforms are applied left-to-right. Each transform processes the
  output of the previous transform.

  Examples:
    # Split array, then filter with JSONPath
    pj split '$[?@.age > 18]' < users.json

    # Extract nested field from all items
    pj '$.users[*]' split '$.address.city' < data.json

    # Get all names at any depth and collect into array
    pj '$..name' join < nested.json

    # Limit depth and convert to JPV
    pj -out jpv depth=2 < deep.json

MORE EXAMPLES:
  For Unix-like usage patterns (head/tail/grep), see:
    pj -help-cookbook

JSONPATH STRICT MODE vs DEFAULT MODE:

  By default, pj optimizes for streaming performance. For descendant queries
  (using ..), results are emitted in document order as items are encountered.
  This allows constant memory usage and immediate output.

  With -jsonpath-strict, pj follows RFC 9535 ordering exactly. For descendant queries,
  all matches at the current level are emitted before descending into nested
  values. This requires buffering collections and reduces streaming efficiency.

  Example with '$..[*]' on {"a": {"b": 1}, "c": 2}:
    Default: outputs in document order: {"b": 1}, 1, 2
    Strict:  outputs per-level first: {"b": 1}, 2, 1

  Use -jsonpath-strict when:
  - You need RFC 9535 compliance for interoperability
  - Output order matters for your use case
  - You're processing small files where memory isn't a concern

  Use default (no -jsonpath-strict) when:
  - You want maximum streaming performance
  - You're processing large files or infinite streams
  - Document order is acceptable

NOTES:
  - Always use single quotes around JSONPath expressions
  - The $ refers to the root of the current value
  - The @ in filters refers to the current node being filtered
  - JSONPath queries preserve streaming where possible (especially without -jsonpath-strict)
`)
}

func printCookbookHelp() {
	fmt.Fprint(os.Stderr, `pj - Cookbook: Unix-like JSON Processing

OVERVIEW:
  pj can be used like Unix text tools (head, tail, grep) but for JSON data.
  Unlike tools that load entire files into memory, pj processes JSON as a
  stream, providing constant memory usage and immediate output.

  This means pj works efficiently with:
  - Very large JSON files (gigabytes)
  - Infinite streams (e.g., from 'yes' or streaming APIs)
  - Pipes where you want immediate results (with 'less', 'head', etc.)

APPROACH 1: JSONPath Queries
  Use JSONPath expressions directly to filter and extract data.

APPROACH 2: JPV + Unix Tools
  Convert to JPV format (JSON Path-Value), use standard Unix tools like
  grep/head/tail, then convert back to JSON.

────────────────────────────────────────────────────────────────────────

LIKE 'head' - GET FIRST N ITEMS

  JSONPath approach:
    # Get first 10 items from array
    pj '$.items[:10]' < data.json

    # Get first 5 users
    pj '$.users[:5]' < users.json

  split + head approach:
    # Split array and use Unix head
    pj split | head -5

  Stream-friendly: outputs the first N items immediately, using constant
  memory. When pj knows the input is a single value (a .json file argument,
  or -in json) it stops reading right after the Nth item, even on an
  infinite stream. For a stream of values it skips to the next value.
    pj '$.items[:10]' huge.json
    curl https://api.example.com/items | pj -in json '$.items[:10]'

────────────────────────────────────────────────────────────────────────

LIKE 'tail' - GET LAST N ITEMS

  JSONPath approach:
    # Get last 10 items from array
    pj '$.items[-10:]' < data.json

    # Get last 3 log entries
    pj '$.logs[-3:]' < logs.json

  Memory efficient: Uses sliding window, doesn't load entire array.

────────────────────────────────────────────────────────────────────────

LIKE 'grep' - FILTER/SEARCH DATA

  JSONPath approach:
    # Find users who have an email field
    pj '$.users[?@.email]' < users.json

    # Find expensive items
    pj '$.products[?@.price > 100]' < products.json

    # Find all error logs
    pj '$..[?@.level == "error"]' < logs.json

  JPV + grep approach:
    # Find all fields containing "gmail"
    pj -out jpv < data.json | grep gmail | pj -in jpv

    # Find all price fields
    pj -out jpv < data.json | grep 'price' | pj -in jpv

    # Case-insensitive search
    pj -out jpv < logs.json | grep -i 'error' | pj -in jpv

  Stream-friendly: Processes records one at a time, outputs matches
  immediately without loading entire file.

────────────────────────────────────────────────────────────────────────

COMBINING PATTERNS

  Filter active users:
    pj '$.users[?@.active == true]' < users.json

  Filter expensive items:
    pj '$.products[?@.price > 100]' < products.json

  Extract emails, collect to array:
    pj '$..email' join < data.json

  Split array, truncate depth, search:
    pj split depth=1 | grep -i 'error'

  Convert JSON array to JSON Lines format:
    pj -json-lines split < array.json

────────────────────────────────────────────────────────────────────────

STREAMING EXAMPLES

  Process infinite stream (Ctrl+C to stop):
    yes '{"id": 1, "name": "test"}' | pj

  Pretty-print streaming API response:
    curl -N https://stream-api.example.com | pj

  Process large file with 'less' (immediate output):
    pj < huge.json | less

  Get first 20 lines of pretty output from infinite stream:
    yes '{"x": 1}' | pj | head -20

  Filter a JSON Lines log (join wraps the lines in an array so the filter
  can select from them, and still streams):
    tail -f app.log | pj join '$[?@.level == "error"]' '$.message'

────────────────────────────────────────────────────────────────────────

JPV FORMAT WORKFLOW

  JPV (JSON Path-Value) format is like 'gron' but uses JSONPath syntax.
  It's perfect for grep-style workflows:

  1. Convert to JPV:
     pj -out jpv < data.json

  2. Use Unix tools:
     pj -out jpv < data.json | grep 'email'

  3. Convert back to JSON:
     pj -out jpv < data.json | grep 'email' | pj -in jpv

  Example output (JPV format):
    $.users[0].name = "Alice"
    $.users[0].email = "alice@example.com"
    $.users[1].name = "Bob"
    $.users[1].email = "bob@example.com"

  Grep for specific fields:
    pj -out jpv < data.json | grep '\.email' | pj -in jpv
    # Reconstructs JSON with only email fields

  Remove lines (removes from JSON):
    pj -out jpv < data.json | grep -v 'password' | pj -in jpv
    # Removes all password fields

────────────────────────────────────────────────────────────────────────

MEMORY USAGE NOTES

  pj uses constant memory for streaming operations:
  - '$.items[*]' or '$.items[:N]': Streams items one by one
  - '$.items[-N:]': Uses sliding window (only last N in memory)
  - split: Converts array to stream (one item at a time)
  - join: Wraps stream in array brackets without buffering
  - Filters: Process one record at a time

  Operations that may need more memory:
  - Some JSONPath operations that require lookahead
  - Sorting (not yet implemented)

  This means you can safely run:
    pj '$.items[:100]' < 10GB-file.json

  And it will output the first 100 items almost instantly, using
  constant memory regardless of file size. Pass the file as an argument
  (or use -in json) and it exits as soon as it has them instead of reading
  to the end of the file:
    pj '$.items[:100]' 10GB-file.json
`)
}
