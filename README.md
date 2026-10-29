# chronos

The command-line interface for [Chronos](https://chronos.sh). Create and run schedules, start one-off jobs, and see what ran and what failed. Every command prints JSON with `--json` and exits with a code listed in `chronos help exit-codes`, so scripts and coding agents can drive it as well as people.

## Build from source

You need Go 1.26 or later.

```sh
go build -o chronos ./cmd/chronos
./chronos --help
```

## Development

`make check` runs every check CI runs. Run it before opening a pull request.

## License

[MIT](LICENSE)
