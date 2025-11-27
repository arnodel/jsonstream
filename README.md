# jp - JSON Stream Processor

Stream, filter, and transform JSON with Unix pipeline efficiency.

```bash
# Process gigabyte files efficiently
cat huge.json | jp '$.items[?@.price > 100]' | less

# Monitor live logs in real-time
tail -f app.log | jp '$..[?@.level == "error"]' '$.message'

# Compose with Unix tools using JPV format
jp -out jpv < data.json | grep -v password | sed 's/test/prod/' | jp
```

## What is jp?

`jp` is a command-line JSON processor designed for **Unix pipelines**. It processes JSON as a **stream** with **immediate output**, making it ideal for large files, infinite streams, and real-time data processing.

Unlike tools that load entire files into memory, `jp` starts outputting results immediately and works seamlessly with other Unix tools like `less`, `grep`, `head`, and `tail`. Many operations use constant memory, especially in default streaming mode.

## Why jp?

### Pipeline-First Design

`jp` follows the Unix philosophy: do one thing well and compose with other tools.

- **Immediate output**: Start seeing results right away, don't wait for the full file
- **Memory-efficient streaming**: Many operations use constant memory, especially simple queries and transforms
- **Streaming architecture**: Perfect for `| less`, `| head`, `| grep` and other pipelines
- **Composable**: Chain multiple transforms in a single `jp` call, or mix with standard Unix tools

### Powerful Features

- **Full JSONPath support**: Complete [RFC 9535](https://datatracker.ietf.org/doc/rfc9535/) implementation
- **Unix-like operations**: `head`, `tail`, `grep` patterns for JSON data
- **Format conversion**: JSON, JSON Lines, CSV, and JPV (JSON Path-Value)
- **JPV format**: Makes JSON grep-able and sed-able like plain text

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Key Features](#key-features)
- [Common Use Cases](#common-use-cases)
- [Pipeline Examples](#pipeline-examples)
- [The JPV Format](#the-jpv-format)
- [Advanced Features](#advanced-features)
- [Input/Output Formats](#inputoutput-formats)
- [Command Reference](#command-reference)
- [JSONPath Compliance](#jsonpath-compliance)
- [The jsonstream Package](#the-jsonstream-package)
- [Development Status](#development-status)

## Installation

```bash
go install github.com/arnodel/jsonstream/cmd/jp@latest
```

## Quick Start

### Pretty-Print JSON

```bash
# Format JSON with colors (when outputting to terminal)
cat data.json | jp

# Browse large files with immediate output
cat huge.json | jp | less
```

### Extract Data with JSONPath

```bash
# Get all names
echo '{"users":[{"name":"Alice"},{"name":"Bob"}]}' | jp '$.users[*].name'
# Output:
# "Alice"
# "Bob"

# Filter by condition
echo '[{"price":50},{"price":150}]' | jp '$[?@.price > 100]'
# Output:
# {"price": 150}
```

### Use in Pipelines

```bash
# Get first 10 items (immediate output, stops reading after 10)
jp '$.items[:10]' < data.json

# Get last 5 items (memory-efficient sliding window)
jp '$.items[-5:]' < data.json

# Monitor live log stream
tail -f app.log | jp '$..[?@.level == "error"]'
```

### Convert Formats

```bash
# JSON array to JSON Lines (one item per line)
echo '[1,2,3]' | jp -json-lines split
# Output:
# 1
# 2
# 3

# CSV to JSON
cat data.csv | jp -in csv-with-header
```

## Key Features

### JSONPath Queries

Full [RFC 9535](https://datatracker.ietf.org/doc/rfc9535/) compliant JSONPath implementation with streaming optimization.

```bash
# Basic selectors
jp '$.name'           # Get field
jp '$.items[0]'       # First item
jp '$.items[-1]'      # Last item
jp '$.items[2:5]'     # Slice (items 2, 3, 4)
jp '$.items[*]'       # All items

# Recursive descent
jp '$..email'         # All email fields at any depth

# Filters
jp '$[?@.price < 100]'              # Price less than 100
jp '$.users[?@.active == true]'     # Active users
jp '$.logs[?@.level == "error"]'    # Error logs
```

**Streaming optimization**: By default, `jp` optimizes for streaming performance. Use `-jsonpath-strict` for exact RFC 9535 ordering (less streaming-friendly).

### Unix-Like Operations

Use `jp` like `head`, `tail`, and `grep` but for JSON:

```bash
# HEAD: Get first N items
jp '$.items[:10]' < data.json                    # JSONPath approach
jp split < array.json | head -10                 # Unix pipe approach

# TAIL: Get last N items (memory-efficient sliding window)
jp '$.items[-10:]' < data.json

# GREP: Filter data
jp '$.users[?@.email]' < users.json              # JSONPath filter
jp -out jpv < data.json | grep email | jp       # grep approach (see JPV below)
```

Most common operations stream efficiently with constant or limited memory.

### Format Conversion

Convert between JSON, JSON Lines, CSV, and JPV formats:

```bash
# JSON array to JSON Lines
cat array.json | jp -json-lines split

# CSV to JSON
cat data.csv | jp -in csv-with-header

# JSON to CSV-like structures
jp '$.users[*]' < data.json | jp -json-lines
```

**Supported formats:**
- **JSON**: Standard JSON with pretty-printing
- **JSON Lines**: Newline-delimited JSON (one value per line)
- **CSV**: Comma-separated values with header support
- **JPV**: JSON Path-Value format (see dedicated section below)

## Common Use Cases

### Process Large Files

```bash
# Browse huge file with immediate output (works with any size file)
cat 10GB-file.json | jp | less

# Get first 100 items from huge file (reads only what's needed)
jp '$.items[:100]' < 10GB-file.json

# Extract specific data from large file
jp '$.transactions[?@.amount > 1000].id' < huge-file.json | less
```

### Monitor Real-Time Streams

```bash
# Watch logs in real-time
tail -f application.log | jp '$..[?@.level == "error"]'

# Pretty-print streaming API
curl -N https://stream-api.example.com/events | jp

# Process infinite stream (Ctrl+C to stop)
yes '{"timestamp": "2024-01-01", "value": 42}' | jp | head -20
```

### Extract and Transform Data

```bash
# Extract all email addresses and collect them
jp '$..email' join < data.json

# Get names from nested structure
jp '$.departments[*].employees[*].name' < org.json

# Filter and extract
jp '$.products[?@.inStock == true].name' < inventory.json
```

### Build Complex Pipelines

```bash
# Multi-stage transformation (chain transforms in one jp call)
jp '$.users[*]' split '$.address.city' < data.json | sort | uniq

# Combine with Unix tools
jp -json-lines '$.items[*]' split < data.json | grep -i 'important' | wc -l

# Chain multiple filters
jp '$.logs[*]' split '$[?@.level == "error"]' '$[?@.timestamp > "2024-01-01"]' < logs.json
```

**Why chain transforms in a single `jp` call?**
Using `jp T1 T2` instead of `jp T1 | jp T2` is more efficient: it avoids serialization/deserialization overhead and pipe I/O. The transforms process the token stream directly in memory.

## Pipeline Examples

`jp` shines in Unix pipelines thanks to its streaming architecture.

### Why Streaming Matters

Traditional JSON tools often load entire files before producing output. `jp`'s streaming approach means:

1. **Immediate feedback**: See results right away, don't wait for the full file
2. **Works with infinite streams**: Process data that never ends (logs, APIs)
3. **Memory efficiency**: Many operations use constant or limited memory
4. **Pipeline efficiency**: Each tool in the chain starts working immediately
5. **Early termination**: `| head -10` stops reading after 10 items

### Complex Multi-Stage Examples

```bash
# Extract, transform, and analyze
jp '$.transactions[?@.amount > 100]' split '$.user.email' < data.json | \
  sort | uniq -c | sort -nr | head -10

# Sanitize config with JPV
jp -out jpv < config.json | grep -v 'password' | grep -v 'secret' | jp > sanitized.json
```

## The JPV Format

JPV (JSON Path-Value) is a line-based format that makes JSON compatible with classic Unix text processing tools like `grep`, `sed`, and `awk`.

### What is JPV?

JPV represents JSON as lines of "path = value" pairs using JSONPath syntax:

```bash
# Original JSON
echo '{"name":"Alice","email":"alice@example.com","age":30}' | jp -out jpv

# JPV output
$.name = "Alice"
$.email = "alice@example.com"
$.age = 30
```

### Why JPV?

JPV makes JSON **line-oriented**, unlocking the full power of Unix text tools:

- **grep-friendly**: Search for fields or values like plain text
- **sed-able**: Edit values with familiar text tools
- **awk-compatible**: Process with awk scripts
- **Reversible**: Convert JSON → JPV → JSON without data loss
- **Subset property**: Any subset of JPV lines is still valid JPV

### JPV vs GRON

JPV is similar to [gron](https://github.com/tomnomnom/gron) but uses standard [JSONPath syntax](https://datatracker.ietf.org/doc/rfc9535/) instead of a custom format.

### Basic JPV Workflow

```bash
# 1. Convert JSON to JPV
jp -out jpv < data.json

# 2. Process with Unix tools
jp -out jpv < data.json | grep email

# 3. Convert back to JSON
jp -out jpv < data.json | grep email | jp
```

### JPV Pipeline Examples

#### Find All Email Fields

```bash
# Search for email fields anywhere in the JSON
jp -out jpv < data.json | grep '\.email'

# Find specific email domains
jp -out jpv < users.json | grep 'email.*gmail.com' | jp
```

**Output:**
```json
{
  "users": [
    {
      "email": "alice@gmail.com"
    },
    {
      "email": "bob@gmail.com"
    }
  ]
}
```

#### Remove Sensitive Fields

```bash
# Strip passwords and secrets
jp -out jpv < config.json | grep -v 'password' | grep -v 'secret' | jp

# Remove entire sections
jp -out jpv < data.json | grep -v '\.credentials' | jp
```

**Before:**
```json
{
  "user": "alice",
  "password": "secret123",
  "api_key": "xyz"
}
```

**After:**
```json
{
  "user": "alice",
  "api_key": "xyz"
}
```

#### Edit Values with sed

```bash
# Replace domain in all email addresses
jp -out jpv < users.json | sed 's/@oldcorp\.com/@newcorp.com/g' | jp

# Update environment in config
jp -out jpv < config.json | sed 's/environment.*=.*"dev"/environment = "prod"/' | jp

# Update all port numbers
jp -out jpv < services.json | sed 's/port = 8080/port = 9090/g' | jp
```

#### Complex Multi-Tool Pipelines

```bash
# Find, filter, and transform
jp -out jpv < data.json | \
  grep 'users' | \                # Only user-related fields
  grep -v 'password' | \          # Remove passwords
  sed 's/status = "active"/status = "verified"/' | \  # Update status
  jp                              # Convert back to JSON

# Extract and analyze with awk
jp -out jpv < metrics.json | \
  grep 'count' | \                # Get count fields
  awk -F' = ' '{sum += $2} END {print sum}'  # Sum all counts

# Conditional editing
jp -out jpv < config.json | \
  awk '/timeout = / {gsub(/[0-9]+/, "5000")} {print}' | \  # Set all timeouts to 5000
  jp
```

#### Diff JSON Files

```bash
# Compare two JSON files as text
diff <(jp -out jpv < old.json | sort) <(jp -out jpv < new.json | sort)

# Find what changed
jp -out jpv < old.json | sort > old.jpv
jp -out jpv < new.json | sort > new.jpv
diff -u old.jpv new.jpv
```

### The Subset Property

JPV has a unique property: **any subset of JPV lines (preserving order) is still valid JPV** that can be converted back to JSON.

This means you can:
- Delete any lines (removes those fields from JSON)
- Extract matching lines (creates JSON with only those fields)

**Note:** You must preserve the original order of lines. Reordering JPV lines may produce invalid JSON structure.

```bash
# Original JSON
{"name":"Alice","age":30,"city":"Boston","country":"USA"}

# Convert to JPV and remove some lines
$ echo '{"name":"Alice","age":30,"city":"Boston","country":"USA"}' | \
  jp -out jpv | grep -v age | grep -v country | jp
{
  "name": "Alice",
  "city": "Boston"
}
```

This is what makes JPV so powerful for Unix-style text processing of JSON data.

## Advanced Features

### Transform Chaining

Chain multiple transforms in a single `jp` call for efficiency:

```bash
# Each transform processes the output of the previous one
jp split '$[?@.age > 18]' '$.name' join < data.json

# Limit depth, split, and extract
jp depth=1 split '$.summary' < data.json
```

**Available transforms:**
- `split`: Split array into stream of values
- `join`: Join stream of values into array
- `depth=N`: Truncate output at depth N
- `trace`: Log stream to stderr (debugging)

**Note:** Multiple transforms in one `jp` call is more efficient than piping through multiple `jp` processes, as it avoids serialization overhead.

### Streaming Behavior

`jp` processes JSON as a **stream of tokens**, not as complete in-memory objects. This enables efficient processing of large files and infinite streams.

**Streaming examples:**

```bash
# Get first 100 items from huge file (stops reading after 100)
jp '$.items[:100]' < 10GB-file.json

# Process infinite input (Ctrl+C to stop)
yes '{"x": 1}' | jp | head -20

# Start outputting immediately, no waiting
cat huge.json | jp | less
```

**Memory-efficient operations:**
- `$.items[*]`: Streams one item at a time (constant memory)
- `$.items[:N]`: Stops after N items (constant memory)
- `$.items[-N:]`: Sliding window (N items in memory)
- `split`: Converts array to stream (constant memory)
- `join`: Wraps stream in brackets (no buffering)
- Simple filters like `$[?@.price > 100]`: Process one item at a time

**Operations that may use more memory:**
- Strict mode (`-jsonpath-strict`) with descendant queries: buffers collections for RFC ordering
- Complex filters with multiple paths or function calls: may need to buffer values
- Queries with negative indices without lookahead optimization
- Very deeply nested descendant queries (`$..`)

For maximum memory efficiency, use default (non-strict) mode and prefer simple queries.

### Strict Mode vs Default Mode

By default, `jp` optimizes for streaming. Use `-jsonpath-strict` for exact RFC 9535 ordering.

**Default mode (streaming-optimized):**
- Results emitted in document order as encountered
- Descendant queries (`$..`) process items immediately
- Constant memory for most operations
- Best for: large files, infinite streams, real-time data

**Strict mode (`-jsonpath-strict`):**
- Results emitted in RFC 9535 order
- Descendant queries process level-by-level
- May buffer collections
- Best for: RFC compliance, reproducible ordering, small files

**Example with `$..[*]` on `{"a": {"b": 1}, "c": 2}`:**
- Default: `{"b": 1}, 1, 2` (document order)
- Strict: `{"b": 1}, 2, 1` (per-level order)

Use `-jsonpath-strict` when:
- You need RFC 9535 compliance for interoperability
- Output order matters for your use case
- Processing small files where memory isn't a concern

Use default (no `-jsonpath-strict`) when:
- You want maximum streaming performance
- Processing large files or infinite streams
- Document order is acceptable

## Input/Output Formats

### Input Formats (`-in FORMAT`)

| Format | Description | Example |
|--------|-------------|---------|
| `auto` (default) | Auto-detect format | `jp < data.json` |
| `json` | Standard JSON, supports JSON Lines | `jp -in json < data.json` |
| `jpv` | JSON Path-Value format | `jp -in jpv < data.jpv` |
| `csv` | CSV records as JSON arrays | `jp -in csv < data.csv` |
| `csvh` or `csv-with-header` | CSV with header row | `jp -in csvh < data.csv` |

**CSV options:**
- `-csv-header NAMES`: Specify field names for CSV (comma-separated)
- CSV values: numbers, booleans (`true`/`false`), and `null` are parsed
- Empty CSV fields become `null`

### Output Formats (`-out FORMAT`)

| Format | Description | Example |
|--------|-------------|---------|
| `json` (default) | Pretty-printed JSON | `jp < data.json` |
| `jpv` | JSON Path-Value format | `jp -out jpv < data.json` |

**JSON output options:**
- `-json-lines` or `-json-compact`: Output JSON Lines (one value per line)
- `-json-indent N`: Indentation level (default: 2)
- `-json-compact-width N`: Max width for inline arrays/objects (default: 60)
- `-color MODE`: Color output (`auto`, `always`, `never`)

**JPV output options:**
- `-jpv-quote-keys`: Always quote keys in brackets

## Command Reference

`jp` has comprehensive built-in help:

```bash
jp -h                    # Main help
jp -help-input           # Detailed input format help
jp -help-output          # Detailed output format help
jp -help-transforms      # Transform help with examples
jp -help-cookbook        # Unix-like usage patterns (head/tail/grep)
```

**Common flags:**
```bash
-in FORMAT               # Input format: json, jpv, csv, csvh, auto (default: auto)
-out FORMAT              # Output format: json, jpv (default: json)
-json-lines              # Output JSON Lines format
-json-indent N           # Indentation level (default: 2)
-jsonpath-strict         # Strict RFC 9535 mode
-color MODE              # Color output: auto, always, never
```

**Example usage:**
```bash
jp [options] [transforms...] < input.json

# Multiple transforms
jp '$.users[*]' split '$.email' < data.json

# With options
jp -json-lines -color never '$.items[*]' < data.json
```

## JSONPath Compliance

`jp` implements the full [RFC 9535](https://datatracker.ietf.org/doc/rfc9535/) JSONPath specification. The implementation passes all tests from the [official JSONPath compliance test suite](https://github.com/jsonpath-standard/jsonpath-compliance-test-suite) (as of 2025-11-23).

By default, `jp` optimizes for streaming performance while maintaining compliance with query semantics. Use `-jsonpath-strict` for exact RFC 9535 ordering requirements.

## The jsonstream Package

`jp` is built on the `jsonstream` Go package, which provides:

- **Token-based streaming**: Process JSON without loading full documents
- **JSONPath implementation**: Full RFC 9535 support
- **Format encoders/decoders**: JSON, JPV, CSV
- **Transform pipeline**: Composable stream transformations

If you're building Go applications that need streaming JSON processing, check out the [package documentation](https://pkg.go.dev/github.com/arnodel/jsonstream).

## Development Status

`jp` is in active development. The core features are functional and the JSONPath implementation is complete, but I work on it in my spare time so progress is incremental.

**Current status:**
- ✅ Full RFC 9535 JSONPath implementation
- ✅ Streaming architecture (many operations use constant memory)
- ✅ JSON, JSON Lines, CSV, and JPV formats
- ✅ Unix-like operations (head/tail/grep patterns)
- ✅ Transform chaining
- ⏳ Additional transforms and features planned

Feedback and contributions welcome! Please [open an issue](https://github.com/arnodel/jsonstream/issues) if you find bugs or have feature requests.

## License

See [LICENSE](LICENSE) file for details.
