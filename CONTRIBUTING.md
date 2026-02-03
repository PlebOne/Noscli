# Contributing to Noscli

Thank you for your interest in contributing to Noscli! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR-USERNAME/Noscli.git`
3. Create a new branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Test your changes thoroughly
6. Commit with a descriptive message
7. Push to your fork
8. Open a Pull Request

## Development Setup

### Prerequisites
- Go 1.25+
- Linux operating system
- Pleb Signer (optional, for Pleb Signer authentication testing)

### Building
```bash
go build -o noscli .
```

### Running
```bash
./noscli
```

## Code Style

- Follow standard Go formatting (`gofmt`)
- Write clear, descriptive commit messages
- Comment complex logic
- Keep functions focused and concise

## Pull Request Guidelines

- **One feature per PR**: Keep PRs focused on a single feature or bug fix
- **Clear description**: Explain what your PR does and why
- **Test your changes**: Ensure everything works before submitting
- **Update documentation**: If you change functionality, update the README
- **Follow existing patterns**: Match the existing code style and architecture

## Areas for Contribution

We welcome contributions in these areas:
- Bug fixes
- Performance improvements
- UI/UX enhancements
- New features (discuss in an issue first)
- Documentation improvements
- Test coverage
- Additional Nostr NIPs support

## Reporting Bugs

Open an issue with:
- Clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Your environment (OS, Go version, etc.)
- Relevant logs or error messages

## Feature Requests

Open an issue describing:
- The feature you'd like
- Why it would be useful
- How you envision it working

## Code of Conduct

- Be respectful and considerate
- Welcome newcomers
- Focus on constructive feedback
- Assume good intentions

## Questions?

Feel free to open an issue with the "question" label or reach out on Nostr!

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
