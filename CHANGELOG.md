# Changelog

All notable changes to promptshell are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.1](https://github.com/oluwatayo/promptshell/compare/v0.6.0...v0.6.1) (2026-07-13)


### Bug Fixes

* replace retired gemini-pro default with gemini-flash-latest ([5719b96](https://github.com/oluwatayo/promptshell/commit/5719b96c1c318e4b2a4e5c21bdca96d297576eff))
* replace retired gemini-pro default with gemini-flash-latest ([a76a5e7](https://github.com/oluwatayo/promptshell/commit/a76a5e7b6bddf417056fed8284d3d53839b30068))

## [0.6.0](https://github.com/oluwatayo/promptshell/compare/v0.5.0...v0.6.0) (2026-07-12)


### Features

* create a pshell alias symlink in install.sh ([766f410](https://github.com/oluwatayo/promptshell/commit/766f41008dce6e9f93ebc52e6b76b4c8bffb6091))
* pshell alias symlink and README command polish ([048a8fe](https://github.com/oluwatayo/promptshell/commit/048a8fe153a428c3d20e8ae9d2fdb16ea17eb953))

## [0.5.0](https://github.com/oluwatayo/promptshell/compare/v0.4.0...v0.5.0) (2026-07-12)


### Features

* add --update flag to self-update from GitHub releases ([414fe73](https://github.com/oluwatayo/promptshell/commit/414fe73605414989cc7fb4d26219ee4e3366b9a6))
* add -v shorthand and go-install version fallback ([3c28f4e](https://github.com/oluwatayo/promptshell/commit/3c28f4ee1616286db62abb55fda59b9753416efb))
* download release assets with progress display ([0b634d5](https://github.com/oluwatayo/promptshell/commit/0b634d53005f5dac5228e90323ecedaffbf9f7bb))
* resolve latest release version for self-update ([6ab0bad](https://github.com/oluwatayo/promptshell/commit/6ab0bad1428b01b9835901624c91b8589d8d7b51))
* self-update flow for promptshell --update ([8cfdf5a](https://github.com/oluwatayo/promptshell/commit/8cfdf5a832b78cc156f7b2af5de1870cc39bded5))
* show a download progress bar in install.sh on a TTY ([e303284](https://github.com/oluwatayo/promptshell/commit/e3032842f89170b295b4cf4ec9bf085f439a3aff))
* verify checksums and atomically replace the binary ([e6e6f2e](https://github.com/oluwatayo/promptshell/commit/e6e6f2ea99cf0220e1cd2a8da2766298be0f2417))
* version/update flags and installer download progress ([1ec2808](https://github.com/oluwatayo/promptshell/commit/1ec2808335d4355b77e1ce9dd5028ff450a4660b))


### Bug Fixes

* pin golang.org/x/mod to keep the Go 1.24 floor ([c0ab734](https://github.com/oluwatayo/promptshell/commit/c0ab734572a2ea9b04419d50536b8fa8e14db02c))
* refuse self-update on VCS-stamped source builds ([421555a](https://github.com/oluwatayo/promptshell/commit/421555a52e4a5cb1beaf17a73030ca0aa4e45f79))

## [0.4.0](https://github.com/oluwatayo/promptshell/compare/v0.3.0...v0.4.0) (2026-07-12)


### Features

* offer to install and start Ollama on first run ([2e296fc](https://github.com/oluwatayo/promptshell/commit/2e296fc2a6f301e7fed586adb2a9a2f3d94f0b1a))
* offer to install and start Ollama on first run ([51cc98f](https://github.com/oluwatayo/promptshell/commit/51cc98f5bcfa99cb20e05da66025f9006da989bf))

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
