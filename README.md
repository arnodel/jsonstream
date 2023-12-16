## JSONStream

My latest weekend project.  Implements a `jp` CLI utility that parses
[JSON](https://www.json.org/) or [JSON Lines](https://jsonlines.org/) input and
processes it in a streaming manner, notably using [JSONPath
queries](https://datatracker.ietf.org/doc/draft-ietf-jsonpath-base/). It has
some of the properties of `cat`, `head`, `tail`, `grep` but for structured JSON
input rather than lines of text.

- Like `cat` and `grep` it can handle infinite inputs and starts outputting very
  quickly, so it works well as part of a pipeline;
- like `tail` it can give you the last few items of a documents in a memory
  efficient way;
- like `grep` it can filter input documents in a streaming manner;
- like `head` it can give you the first few items of an infinite input.

On top of that:
- it can ingest and output CSV documents (so you can convert JSON <-> CSV for
  instance)
- it can prettify JSON in a streaming way (useful for piping to `less`)
- input can be filtered with JSONPath expressions - and the full (*) [JSONPath
  spec](https://datatracker.ietf.org/doc/html/draft-ietf-jsonpath-base-21) is
  implemented in a way that tries to preserve the streaming properties of the
  utility whenever possilbe

Note that the streaming aspect is not limited to the toplevel items (e.g. items
in a JSON array or in a JSON Lines input) but applies to arbitrary levels of nesting
in the JSON input.  There are examples of what this means below.

Having said that, it's still in early stages and progress is haphazard as I only
work on it in my spare time.  The rest of the README below is not very well
organised yet, but you can still read on for more details.

(*) not quite true: slices with negative step are not yet implemented (e.g.
`$[::-1]`).

## The `jsonstream` package

It can decode JSON into a stream, apply transformers to the stream, and
encode a stream back into JSON. It aims to be able to scale to arbitrarily
large input without increasing memory usage or latency.

It contains a full implementation of the latest (at the time of writing)
JSONPath draft spec
(https://datatracker.ietf.org/doc/html/draft-ietf-jsonpath-base-21). This is
used to implement JSONPath-based transformers.

It's been
made for the CLI utility below. So it tries to provide tools to make it easy to
implement the "transformers" mentioned below.

## The `jp` CLI utility

It stands for "Json Processor" or perhaps "Json Path". Install with

```
go install github.com/arnodel/jsonstream/cmd/jp
```

Use it like this:

```
cat somefile.json | jp
```

This will output the json in a nice indented format, just like `jq` would. The
difference that if you run

```
cat a_huge_file.json | jp | less
```

You will get to see stuff straight away, regardless of the size of the file
(same with e.g. `head`).

The `jp` tool automatically handles JSON Lines input. You can change the indentation
level with the `-indent` flag. Set to a positive number, 0 for no indentation, a
negative number will cause `jp` to output everything on one line, saving you
precious vertical space.

But that's not it. There are a number of _transforms_ that are available, and
they can be chained!

Here is a list of transforms:

- a JSONPath expression starting with `$`, e.g `$[-10:].foo` or
  `$..parent.children[10:]`, etc.  The whole draft IETF spec for JSONPath is
  implemented, but the implementation is not settled yet.  More documentation
  needs writing for this as it's becoming the main feature of the command.
- `depth=<n>`: truncate output below a certain depth. E.g. `depth=1` will not
  expand nested arrays or object.
- `.<key>`: just output the value associated with a key, e.g. `.id`.  This is
  becoming obsolete as it can be replaced with the JSONPath expressions `$.key`
  or `$["key"]`.
- `...<key>`: just output the values associated with a key, but it can be at any
  depth in the input. So the result may be a stream of values (as the key may be
  repeated).  This is also becoming obsolete as it can be replaced with the
  JSONPath expressions `$..key` or `$..["key"]`
- `split`: splits an array into a stream of values
- `join`: the reverse, joins a stream of values into an array
- `trace`: (for debugging) eat up the stream and log it to stderr

See the file [builtintransformers.go](builtintransformers.go) for some more
details. There are not many so far but it's easy to add some more, and I'm
planning to do that.

You can choose an input format with the `-in` option:

- `json` selects JSON format
- `jpv` or `path` selects the `JPV` format. It's related (but not quite the same
  :-|) as the format described in https://github.com/tomnomnom/gron. This allows
  a workflow of the type `jp` -out jpv | grep | jp -in jpv` (`-in jpv` is not
  required because the input format should be guessed correctly)
- `csv` selects the `CSV` format.  Each CSV record is streamed as an array of values
  E.g. the following input
  ```
  first_name,last_name,age
  John,Doe,33
  Arnaud,Delobelle,7
  ```
  is streamed as
  ```
  ["first_name", "last_name", "age"]
  ["John", "Doe", 33]
  ["Arnaud", "Delobelle", 7]
  ```
- `csv-header` or `csvh` selects the `CSV` format too, but the first record is considered
  to be a header, so that each subsequent record is streamed as an object.
  E.g. the following input
  ```
  first_name,last_name,age
  John,Doe,33
  Arnaud,Delobelle,7
  ```
  is streamed as
  ```
  {"first_name": "John", "last_name": "Doe", "age": 33} 
  {"first_name": "Arnaud", "last_name": "Delobelle", "age": 7} 
  ```
- `auto` (the default value) tries to guess the format, falling back to JSON if
  it can't

You can choose the output format with the `-out` option.  The available formats
are:

- `json` (the default)
- `jpv` or `path`

### The `JPV` format

It stands for JsonPath-Value.  it's similar to `GRON` (see
https://github.com/tomnomnom/gron) but the paths use the JSONPath format
instead.

```
$ echo '{"name": "Tom", "pets": ["dog", "tortoise", "spider"], "cars": []}' | jp -out jpv
$["name"] = "Tom"
$["pets"][0] = "dog"
$["pets"][1] = "tortoise"
$["pets"][2] = "spider"
$["cars"] = []
```

It has this useful property: if you remove any number of lines from a JPV
stream, you still get a valid JPV stream (i.e. it can be turned back into some
valid JSON).  E.g.

```
echo '{"name": "Tom", "pets": ["dog", "tortoise", "spider"], "cars": []}' | jp -out jpv | grep sp | jp
{
  "pets": [
    "spider"
  ]
}
```

### Examples

Say we have a `sample.json` file with this content:

```json
[
  {
    "id": 3,
    "name": "Tom",
    "labels": {
      "age": 67,
      "friendly": false
    }
  },
  {
    "id": 7,
    "name": "Kim",
    "labels": {
      "age": 2,
      "innocent": true
    },
    "friends": { "name": "Jon" }
  }
]
```

```
$ cat sample.json | jp split depth=1
{
  "id": 3,
  "name": "Tom",
  "labels": {...}
}
{
  "id": 7,
  "name": "Kim",
  "labels": {...},
  "friends": {...}
}
```

```
$ cat sample.json | jp split .name
"Tom"
"Kim"
```

```
$ cat sample.json | jp ...name join
[
  "Tom",
  "Kim",
  "Jon"
]
```

This last example

- outputs the sample file _forever_ on `stdout`
- pipes this runs `jp` to join all the records together in one single array (limiting the depth for a twist),
- pipes this to `head` to get the first 20 lines

And yet it works!

```
$ time yes "$(jp -file sample.json -indent -1)" | jp split depth=1 join | head -20
[
  {
    "id": 3,
    "name": "Tom",
    "labels": {...}
  },
  {
    "id": 7,
    "name": "Kim",
    "labels": {...},
    "friends": {...}
  },
  {
    "id": 3,
    "name": "Tom",
    "labels": {...}
  },
  {
    "id": 7,
    "name": "Kim",
yes "$(jp -file sample.json -indent -1)"  0.00s user 0.00s system 21% cpu 0.014 total
jp split depth=1 join  0.01s user 0.01s system 96% cpu 0.012 total
head -20  0.00s user 0.00s system 26% cpu 0.010 total
```

One last example

```
$ yes '{"name": "bob", "children": ' | jp | head -20
{
  "name": "bob",
  "children": {
    "name": "bob",
    "children": {
      "name": "bob",
      "children": {
        "name": "bob",
        "children": {
          "name": "bob",
          "children": {
            "name": "bob",
            "children": {
              "name": "bob",
              "children": {
                "name": "bob",
                "children": {
                  "name": "bob",
                  "children": {
                    "name": "bob",
```

For now that's all I've got to say. This doesn't look too impressive but it works on very large
files too!
