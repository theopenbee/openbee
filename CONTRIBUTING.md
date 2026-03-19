# Contributing to OpenBee

Thank you for your interest in contributing to OpenBee! This guide will help you get started.

## Contributor License Agreement

By submitting a contribution, you agree to the terms of our [Contributor License Agreement (CLA)](CLA.md) ([中文版](CLA_zh.md)).

## Getting Started

### Prerequisites

- **Go** 1.25+
- **Node.js** 18+ and **pnpm**
- **Git**

### Clone and Build

```bash
git clone https://github.com/theopenbee/openbee.git
cd openbee

# Build frontend assets
make web

# Build the binary
make build
```

The binary will be output to `dist/openbee`.

### Configuration

```bash
./dist/openbee config
```

Run `openbee config` to interactively generate `config.yaml` with all available options.

### Running Tests

```bash
go test ./...
```

## Development Workflow

1. **Fork** the repository and clone your fork
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/your-feature
   ```
3. **Make your changes** and add tests where appropriate
4. **Run tests** to ensure nothing is broken:
   ```bash
   go test ./...
   ```
5. **Commit** your changes with a clear message (see [Commit Messages](#commit-messages))
6. **Push** to your fork and open a **Pull Request** against `main`

## Commit Messages

We follow the [Conventional Commits](https://www.conventionalcommits.org/) style:

```
<type>: <description>

[optional body]
```

**Types:**

| Type       | Description                          |
|------------|--------------------------------------|
| `feat`     | A new feature                        |
| `fix`      | A bug fix                            |
| `docs`     | Documentation changes                |
| `test`     | Adding or updating tests             |
| `refactor` | Code changes that don't fix a bug or add a feature |
| `chore`    | Build process, CI, or tooling changes |
| `ci`       | CI/CD configuration changes          |

**Examples:**

```
feat: add WeChat Work platform support
fix: resolve message deduplication race condition
docs: update configuration guide
```

## Project Structure

```
openbee/
├── cmd/openbee/       # CLI entry point
├── internal/
│   ├── api/           # HTTP API handlers
│   ├── app/           # Application lifecycle
│   ├── bee/           # Core bot logic
│   ├── claude/        # Claude integration
│   ├── config/        # Configuration loading
│   ├── mcp/           # Model Context Protocol
│   ├── platform/      # Platform adapters (Feishu, DingTalk, WeCom)
│   ├── store/         # Data persistence (SQLite)
│   ├── task_dispatcher/ # Background task dispatch
│   ├── task_scheduler/  # Task scheduling
│   └── worker/        # Worker management
├── web/               # Frontend (React + TypeScript)
├── docs/              # Technical documentation
├── Makefile
└── .goreleaser.yml
```

## Pull Request Guidelines

- Keep PRs focused on a single concern
- Include tests for new features and bug fixes
- Make sure all existing tests pass
- Update documentation if your changes affect user-facing behavior
- Reference related issues in the PR description (e.g., `Fixes #123`)

## Reporting Issues

- Use [GitHub Issues](https://github.com/theopenbee/openbee/issues) to report bugs or request features
- Search existing issues before creating a new one
- Include reproduction steps, expected behavior, and actual behavior for bug reports

## Code Style

- Follow standard Go conventions and `gofmt` formatting
- Frontend code should follow the existing ESLint and Prettier configuration in `web/`
- Keep functions focused and well-named; prefer clarity over brevity

## License

By contributing to OpenBee, you agree that your contributions will be licensed under the [MIT License](LICENSE).
