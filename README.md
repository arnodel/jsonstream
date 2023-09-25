## jsonstream

So far, this is a one weekend project...

## The jsonstream package

It can decode JSON into a stream, apply transformers to the stream, and
encode a stream back into JSON. It aims to be able to scale to arbitrarily
large input without increasing memory usage or latency.

It's been made for the CLI utility below. So it tries to provide tools to
make it easy to implement the "transformers" mentioned below.

## The pj CLI utility

It stands for "process json". Install with

```
github.com/arnodel/jsonstream/cmd/pj
```

Use it like this:

```
cat somefile.json | pj
```

This will output the json in a nice indented format, just like `jq` would. The difference
that if you run

```
cat a_huge_file.json | pj | less
```

You will get to see stuff straight away, regardless of the size of the file (same with e.g. `head`).

The `pj` tool automatically handles JSON streams. You can change the indentation level with
the `-indent` flag. Set to a positive number, 0 for no indentation, a negative number will cause
`pj` to output everything on one line, saving you precious vertical space.

But that's not it. There are a number of _transforms_ that are available, and they can be chained!

Here is a list of transforms:

- `depth=<n>`: truncate output below a certain depth. E.g. `depth=1` will not expand nested arrays or object.
- `.<key>`: just output the value associated with a key, e.g. `.id`
- `...<key>`: just output the values associated with a key, but it can be at
  any depth in the input. So the result may be a stream of values (as the key
  may be repeated).
- `split`: splits an array into a stream of values
- `join`: the reverse, joins a stream of values into an array
- `trace`: (for debugging) eat up the stream and log it to stderr

See the file [builtintransformers.go](builtintransformers.go) for some more details. There are not many so far but it's easy to add some more, and I'm planning to do that.

You can choose an input format and an output format with the `-in` and `-out`.

- `json` (the default) selects JSON format
- `gron` selects the `GRON` format. It's related (but not quite the same :-|) as the format described in https://github.com/tomnomnom/gron. This allows a workflow of the type `pj -out gron | grep | pj -in gron`

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
$ cat sample.json | pj split depth=1
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
$ cat sample.json | pj split .name
"Tom"
"Kim"
```

```
$ cat sample.json | pj ...name join
[
  "Tom",
  "Kim",
  "Jon"
]
```

This last example

- outputs the sample file _forever_ on `stdout`
- pipes this runs `pj` to join all the records together in one single array (limiting the depth for a twist),
- pipes this to `head` to get the first 20 lines

And yet it works!

```
$ time yes "$(pj -file sample.json -indent -1)" | pj split depth=1 join | head -20
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
yes "$(pj -file sample.json -indent -1)"  0.00s user 0.00s system 21% cpu 0.014 total
pj split depth=1 join  0.01s user 0.01s system 96% cpu 0.012 total
head -20  0.00s user 0.00s system 26% cpu 0.010 total
```

One last example

```
$ yes '{"name": "bob", "children": ' | pj | head -20
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
