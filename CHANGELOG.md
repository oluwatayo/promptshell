# Changelog

All notable changes to promptshell are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0](https://github.com/oluwatayo/promptshell/compare/v0.2.0...v0.3.0) (2026-06-24)


### Features

* friendly first-run guidance when Ollama isn't available ([89a07ef](https://github.com/oluwatayo/promptshell/commit/89a07ef80899d5811faf7c5084d6bec91c26f6e2))
* friendly first-run guidance when Ollama isn't available ([638aa3e](https://github.com/oluwatayo/promptshell/commit/638aa3ea72e75139ff45f5520f7ada2eb31fc3e6))

## [0.2.0](https://github.com/oluwatayo/promptshell/compare/v0.1.0...v0.2.0) (2026-06-21)


### Features

* add curl|sh install script ([5c85452](https://github.com/oluwatayo/promptshell/commit/5c8545214f1f83ac77b718114aee2d932bda0772))
* curl|sh install script ([352d844](https://github.com/oluwatayo/promptshell/commit/352d844a6f707fab10caf65c0aa30721678252ef))

## [Unreleased]

## [0.1.0] - 2026-06-21

First tagged release.

### Added

- Natural-language → shell-script generation that previews the script and asks
  for confirmation before running it (`--dry-run`, `--yes`).
- Multiple LLM providers behind a common interface: **Ollama** (local, the
  default — no API key), **Gemini**, **OpenAI**, and **Anthropic**.
- Provider/model selection via `--provider` / `--model` flags,
  `PROMPTSHELL_PROVIDER` / `PROMPTSHELL_<PROVIDER>_API_KEY` environment
  variables, and a `config` subcommand backed by `~/.promptshell/config`.
- Interactive shell (run with no task) with `:`-prefixed meta-commands.
- Configurable shell via `--shell` / `PROMPTSHELL_SHELL` (default `bash`).
- `--version` flag.

[Unreleased]: https://github.com/oluwatayo/promptshell/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/oluwatayo/promptshell/releases/tag/v0.1.0
