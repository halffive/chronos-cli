#!/bin/sh
set -eu

dir=dist/completions
rm -rf "$dir"
mkdir -p "$dir"
go run ./cmd/chronos completion bash >"$dir/chronos.bash"
go run ./cmd/chronos completion zsh >"$dir/chronos.zsh"
go run ./cmd/chronos completion fish >"$dir/chronos.fish"
go run ./cmd/chronos completion powershell >"$dir/chronos.ps1"
