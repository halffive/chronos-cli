# chronos

The command-line interface for [Chronos](https://chronos.sh). Create and run schedules, start one-off jobs, and see what ran and what failed. Every subcommand of `schedules`, `jobs`, `executions` and `auth` prints JSON with `--json`, and every command exits with a code listed in `chronos help exit-codes`, so scripts and coding agents can drive it as well as people.

## Install

On macOS or Linux, install with Homebrew:

```sh
brew install halffive/tap/chronos-cli
```

Homebrew builds `chronos` from source, so it installs Go as a build dependency. It also installs the bash, zsh and fish completions.

To install without Homebrew, run the install script:

```sh
curl -fsSL https://chronos.sh/install.sh | sh
```

The script installs the latest release to `~/.local/bin`. To install somewhere else, set `CHRONOS_INSTALL_DIR` on the `sh` side of the pipe:

```sh
curl -fsSL https://chronos.sh/install.sh | CHRONOS_INSTALL_DIR=~/bin sh
```

To install a specific version, pass it as an argument:

```sh
curl -fsSL https://chronos.sh/install.sh | sh -s -- v0.1.0
```

The script verifies the download's sha256 against the release's `checksums.txt`, and installs nothing if the check fails or cannot run. It never edits your shell profile. If the directory is not on your `PATH`, it prints the `export PATH` line to add.

To install with Go 1.26 or later:

```sh
go install chronos.sh/cli/cmd/chronos@latest
```

Archives for macOS, Linux and Windows are on the [releases page](https://github.com/halffive/chronos-cli/releases), with a `checksums.txt` to verify them. Each archive has the `chronos` binary and completion scripts in `completions/`.

Check the install:

```sh
chronos version
```

## Upgrade

Upgrade the same way you installed. `chronos` does not check for updates, so watch the [releases page](https://github.com/halffive/chronos-cli/releases) for new versions.

- Homebrew: run `brew upgrade chronos-cli`.
- Install script: run `curl -fsSL https://chronos.sh/install.sh | sh` again. If you set `CHRONOS_INSTALL_DIR`, set it to the same directory.
- Go: run `go install chronos.sh/cli/cmd/chronos@latest`.
- Release archive: download the new archive from the releases page. Replace the old binary with the one inside.

Your saved API key and config stay in place.

## Uninstall

1. Remove your saved API key from the OS keychain and the config file:

   ```sh
   chronos auth logout
   ```

2. Remove the binary. For a Homebrew install, run `brew uninstall chronos-cli`. Otherwise, delete it:

   ```sh
   rm "$(command -v chronos)"
   ```

3. Remove the config directory, `~/.config/chronos` by default:

   ```sh
   rm -rf ~/.config/chronos
   ```

   If you set `CHRONOS_CONFIG_DIR`, remove that directory instead. If you set `XDG_CONFIG_HOME`, remove `$XDG_CONFIG_HOME/chronos`. On Windows, the default is `%AppData%\chronos`.

## Build from source

You need Go 1.26 or later.

```sh
go build -o chronos ./cmd/chronos
./chronos --help
```

## Development

`make check` runs every check CI runs. Run it before opening a pull request. It downloads the pinned [shellcheck](https://github.com/koalaman/shellcheck) into `dist/tools`. GoReleaser needs Go 1.27.3, which Go fetches for you unless you set `GOTOOLCHAIN=local`.

## License

[MIT](LICENSE)
