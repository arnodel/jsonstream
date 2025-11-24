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
	"github.com/arnodel/jsonstream/encoding/jpv"
	"github.com/arnodel/jsonstream/encoding/json"
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
	flag.BoolVar(&jsonCompact, "json-compact", false, "output JSON on a single line")
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
	var deprecatedFile string
	flag.StringVar(&deprecatedFile, "file", "", "DEPRECATED: use shell redirection (< file)")

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

func printUsage() {
	fmt.Fprint(os.Stderr, `jp - JSON stream processor

USAGE:
  jp [options] [transforms...] < input.json

DESCRIPTION:
  jp processes JSON input in a streaming manner, allowing you to transform,
  filter, and format JSON data with constant memory usage.

  Input is read from stdin. Use shell redirection to read from files:
    jp < file.json
    cat file.json | jp

HELP OPTIONS:
  -help-input       Show detailed help for input formats
  -help-output      Show detailed help for output formats and options
  -help-transforms  Show detailed help for transforms with examples
  -help-cookbook    Show cookbook with Unix-like usage patterns (head/tail/grep)

INPUT/OUTPUT:
  -in FORMAT        Input format (default: auto)
                    Formats: json, jpv, csv, csv-with-header (or csvh)
  -out FORMAT       Output format (default: json)
                    Formats: json, jpv
  -csv-header NAMES Comma-separated field names for CSV input
                    Only valid with '-in csv'

JSON OUTPUT OPTIONS:
  -json-compact         Output JSON on a single line
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
  -strict           Execute JSONPath queries in strict mode (RFC-compliant ordering,
                    less streaming-friendly for descendant queries like $..))

EXAMPLES:
  # Pretty-print JSON
  cat data.json | jp

  # Extract specific field from array items
  cat users.json | jp '$.users[*].name'

  # Filter and transform
  cat data.json | jp '$.items[?@.price < 100]' split

  # CSV to JSON
  cat data.csv | jp -in csv-with-header

  # Compact output
  cat data.json | jp -json-compact

For Unix-like usage patterns (head/tail/grep), see: jp -help-cookbook
For more information, visit: https://github.com/arnodel/jsonstream
`)
}

func printInputHelp() {
	fmt.Fprint(os.Stderr, `jp - Input Format Help

INPUT FORMAT SELECTION:
  Use the '-in FORMAT' flag to specify the input format.
  Default is 'auto', which attempts to detect the format automatically.

AVAILABLE FORMATS:

  json
    Standard JSON format. Supports both single JSON values and JSON Lines
    (newline-delimited JSON values).

    Example:
      {"name": "Alice", "age": 30}
      {"name": "Bob", "age": 25}

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
    Attempts to automatically detect the format based on the first few
    bytes of input. Falls back to JSON if detection fails.

NOTES:
  - Empty fields in CSV are converted to null
  - CSV values 'true' and 'false' are converted to booleans
  - CSV numeric values are parsed as JSON numbers
  - Use shell redirection to read from files: jp -in csv < data.csv
`)
}

func printOutputHelp() {
	fmt.Fprint(os.Stderr, `jp - Output Format Help

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

    -json-compact
      Output everything on a single line.
      Example: {"name": "Alice", "scores": [95, 87, 92], ...}

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
      cat data.json | jp -out jpv | grep city | jp -in jpv

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
  # Compact JSON output
  cat data.json | jp -json-compact

  # No colors even in terminal
  cat data.json | jp -color never

  # Convert JSON to JPV and filter
  cat users.json | jp -out jpv | grep email
`)
}

func printTransformsHelp() {
	fmt.Fprint(os.Stderr, `jp - Transform Help

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

    Use -strict flag for strict mode (RFC-compliant ordering).

  split
    Splits an array into a stream of its individual values.
    Non-array values pass through unchanged.

    Example:
      Input:  [1, 2, 3]
      Output: 1
              2
              3

  join
    Joins a stream of values into a single array.

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
    jp split '$[?@.age > 18]' < users.json

    # Extract nested field from all items
    jp '$.users[*]' split '$.address.city' < data.json

    # Get all names at any depth and collect into array
    jp '$..name' join < nested.json

    # Limit depth and convert to JPV
    jp -out jpv depth=2 < deep.json

MORE EXAMPLES:
  For Unix-like usage patterns (head/tail/grep), see:
    jp -help-cookbook

JSONPATH STRICT MODE vs DEFAULT MODE:

  By default, jp optimizes for streaming performance. For descendant queries
  (using ..), results are emitted in document order as items are encountered.
  This allows constant memory usage and immediate output.

  With -strict, jp follows RFC 9535 ordering exactly. For descendant queries,
  all matches at the current level are emitted before descending into nested
  values. This requires buffering collections and reduces streaming efficiency.

  Example with '$..[*]' on {"a": {"b": 1}, "c": 2}:
    Default: outputs in document order: {"b": 1}, 1, 2
    Strict:  outputs per-level first: {"b": 1}, 2, 1

  Use -strict when:
  - You need RFC 9535 compliance for interoperability
  - Output order matters for your use case
  - You're processing small files where memory isn't a concern

  Use default (no -strict) when:
  - You want maximum streaming performance
  - You're processing large files or infinite streams
  - Document order is acceptable

NOTES:
  - Always use single quotes around JSONPath expressions
  - The $ refers to the root of the current value
  - The @ in filters refers to the current node being filtered
  - JSONPath queries preserve streaming where possible (especially without -strict)
`)
}

func printCookbookHelp() {
	fmt.Fprint(os.Stderr, `jp - Cookbook: Unix-like JSON Processing

OVERVIEW:
  jp can be used like Unix text tools (head, tail, grep) but for JSON data.
  Unlike tools that load entire files into memory, jp processes JSON as a
  stream, providing constant memory usage and immediate output.

  This means jp works efficiently with:
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
    jp '$.items[:10]' < data.json

    # Get first 5 users
    jp '$.users[:5]' < users.json

  split + head approach:
    # Split array and use Unix head
    jp split | head -5

  Stream-friendly: Outputs immediately, stops reading after N items.
  Works on infinite streams!

────────────────────────────────────────────────────────────────────────

LIKE 'tail' - GET LAST N ITEMS

  JSONPath approach:
    # Get last 10 items from array
    jp '$.items[-10:]' < data.json

    # Get last 3 log entries
    jp '$.logs[-3:]' < logs.json

  Memory efficient: Uses sliding window, doesn't load entire array.

────────────────────────────────────────────────────────────────────────

LIKE 'grep' - FILTER/SEARCH DATA

  JSONPath approach:
    # Find users who have an email field
    jp '$.users[?@.email]' < users.json

    # Find expensive items
    jp '$.products[?@.price > 100]' < products.json

    # Find all error logs
    jp '$..[?@.level == "error"]' < logs.json

  JPV + grep approach:
    # Find all fields containing "gmail"
    jp -out jpv < data.json | grep gmail | jp -in jpv

    # Find all price fields
    jp -out jpv < data.json | grep 'price' | jp -in jpv

    # Case-insensitive search
    jp -out jpv < logs.json | grep -i 'error' | jp -in jpv

  Stream-friendly: Processes records one at a time, outputs matches
  immediately without loading entire file.

────────────────────────────────────────────────────────────────────────

COMBINING PATTERNS

  Filter active users:
    jp '$.users[?@.active == true]' < users.json

  Filter expensive items:
    jp '$.products[?@.price > 100]' < products.json

  Extract emails, collect to array:
    jp '$..email' join < data.json

  Split array, truncate depth, search:
    jp split depth=1 | grep -i 'error'

────────────────────────────────────────────────────────────────────────

STREAMING EXAMPLES

  Process infinite stream (Ctrl+C to stop):
    yes '{"id": 1, "name": "test"}' | jp

  Pretty-print streaming API response:
    curl -N https://stream-api.example.com | jp

  Process large file with 'less' (immediate output):
    jp < huge.json | less

  Get first 20 lines of pretty output from infinite stream:
    yes '{"x": 1}' | jp | head -20

  Stream logs through multiple filters:
    tail -f app.log | jp '$..[?@.level == "error"]' | jp '$.message'

────────────────────────────────────────────────────────────────────────

JPV FORMAT WORKFLOW

  JPV (JSON Path-Value) format is like 'gron' but uses JSONPath syntax.
  It's perfect for grep-style workflows:

  1. Convert to JPV:
     jp -out jpv < data.json

  2. Use Unix tools:
     jp -out jpv < data.json | grep 'email'

  3. Convert back to JSON:
     jp -out jpv < data.json | grep 'email' | jp -in jpv

  Example output (JPV format):
    $.users[0].name = "Alice"
    $.users[0].email = "alice@example.com"
    $.users[1].name = "Bob"
    $.users[1].email = "bob@example.com"

  Grep for specific fields:
    jp -out jpv < data.json | grep '\.email' | jp -in jpv
    # Reconstructs JSON with only email fields

  Remove lines (removes from JSON):
    jp -out jpv < data.json | grep -v 'password' | jp -in jpv
    # Removes all password fields

────────────────────────────────────────────────────────────────────────

MEMORY USAGE NOTES

  jp uses constant memory for streaming operations:
  - '$.items[*]' or '$.items[:N]': Streams items one by one
  - '$.items[-N:]': Uses sliding window (only last N in memory)
  - split: Converts array to stream (one item at a time)
  - join: Wraps stream in array brackets without buffering
  - Filters: Process one record at a time

  Operations that may need more memory:
  - Some JSONPath operations that require lookahead
  - Sorting (not yet implemented)

  This means you can safely run:
    jp '$.items[:100]' < 10GB-file.json

  And it will output the first 100 items almost instantly, using
  constant memory regardless of file size!
`)
}
