# chronos-cli

Go CLI `chronos`, module `chronos.sh/cli`. Public, MIT.

- Every API call goes through the Go SDK `chronos.sh/go`. `internal/transport` is the only HTTP code.
- `internal/app` holds what commands share; `internal/cmdutil` adapts it to cobra.
- Commands live in `internal/cmd/<noun>`, one file per verb, built with `cmdutil.NewCommand`. They return an `output.View` and never print results, errors or exit codes themselves. `root.Execute` is the only place that prints errors and picks the exit code.
- Cobra is imported only by `internal/cmd`, `internal/cmdutil` and the `internal/clitest` harness. `app`, `config`, `input`, `output`, `exit` and `transport` never import it.
- The stable contract is `--json` output, exit codes and error codes (`internal/exit`), flag names and environment variables. Add to it; never rename or renumber. Tables and tab-separated output may change.
- A flag that sets an API field or query parameter is that name in kebab-case.
- Mutating commands set `Mutates: true` (see `CommandSpec.Mutates`).
- A saved key is only sent to the base URL it was saved for. Plain http is allowed only for localhost.
- No package-level mutable state and no `init()`: every command is built from an `*app.Factory`.
- Tests are external test packages (`package <noun>_test`) using `internal/clitest`: a fake API, fake terminals and golden files (`go test ./... -update` rewrites them). No mocking libraries.
- `make check` must pass before handing work back. `go.mod` stays at the oldest supported Go release and never has a `replace` directive.
- Commits and PR titles are lowercase conventional commits under 60 characters; branches are `{ft|bg|ch}-{issue-key}-{max-3-words}`.
