# checkssl

[![Node.js Version](https://img.shields.io/badge/node-%3E%3D18.0.0-brightgreen.svg)](https://nodejs.org/)
[![License](https://img.shields.io/badge/license-ISC-blue.svg)](LICENSE)

`checkssl` is a modern Node.js CLI tool that checks SSL certificate expiration dates for domains. It provides a clean, tabular output showing when certificates will expire, helping you stay ahead of certificate renewals.

## Features

- ✅ Check single or multiple domains
- 📁 Read domains from files (with comment support)
- 🏠 Auto-load domains from `~/.checkssl` config file
- 📊 Clean, formatted table output
- 🔄 Sort results by expiration date
- ⚡ Fast concurrent certificate checking
- 🚫 Error suppression option
- 🛡️ Robust error handling and validation

## Installation

### Global Installation

```bash
npm install -g git+https://github.com/floriancrusius/checkssl.git#v1.0.0
```

### Local Development

```bash
git clone https://github.com/floriancrusius/checkssl.git
cd checkssl
npm install
npm link  # Makes checkssl available globally
```

## Usage

### Command Line Options

```bash
checkssl [options]

Options:
  -d <domain>    Check a specific domain
  -f <file>      Read domains from a file
  -s             Suppress error messages
  -h             Show help message
```

### Examples

```bash
# Check a single domain
checkssl -d example.com

# Check multiple domains
checkssl -d example.com -d example.org -d google.com

# Check domains from a file
checkssl -f domains.txt

# Mix file and individual domains
checkssl -f domains.txt -d additional-domain.com

# Suppress error messages
checkssl -d example.com -s
```

### Domain File Format

Create a text file with one domain per line. Comments are supported using `#`:

```bash
# Production domains
example.com          # Main website
api.example.com      # API endpoint
cdn.example.com      # CDN

# Development domains
dev.example.com
staging.example.com
```

### Default Configuration

If no options are provided, checkssl will look for `~/.checkssl` in your home directory:

```bash
# Create default config
echo "example.com" > ~/.checkssl
echo "google.com" >> ~/.checkssl

# Now just run
checkssl
```

## Output Format

The tool displays results in a clean table format:

```
===================================
| example.com     | 15.03.2025 |
| google.com      | 22.07.2025 |
| expired.com     |   Error    |
===================================
```

Results are automatically sorted by expiration date (earliest first).

## Development

### Prerequisites

- Node.js >= 18.0.0
- npm or yarn

### Setup

```bash
git clone https://github.com/floriancrusius/checkssl.git
cd checkssl
npm install
```

### Available Scripts

```bash
npm start           # Run the application
npm test            # Run tests
npm run test:watch  # Run tests in watch mode
npm run lint        # Run ESLint
npm run lint:fix    # Fix ESLint issues
npm run format      # Format code with Prettier
npm run build       # Build binary with pkg
```

### Testing

The project includes comprehensive tests using Jest:

```bash
# Run all tests
npm test

# Run tests with coverage
npm test -- --coverage

# Run tests in watch mode during development
npm run test:watch
```

## Error Handling

The tool provides detailed error messages for common issues:

- **Invalid domains**: Validates domain format according to RFC standards
- **Network errors**: Handles connection timeouts and SSL/TLS errors
- **File errors**: Clear messages for missing or unreadable files
- **Certificate errors**: Detects expired certificates and missing certificate data

Use the `-s` flag to suppress error messages in scripts or automated environments.

## Building

Create standalone binaries for distribution:

```bash
# Build for current platform
npm run build

# Build for all supported platforms (requires make)
make build
```

Binaries are created in the `dist/` directory.

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Make your changes and add tests
4. Run the test suite: `npm test`
5. Check code formatting: `npm run lint`
6. Commit your changes: `git commit -am 'Add feature'`
7. Push to the branch: `git push origin feature-name`
8. Submit a pull request

## License

ISC License - see [LICENSE](LICENSE) file for details.

## Changelog

### v1.0.0

- Initial release
- Support for single and multiple domain checking
- File-based domain input with comment support
- Clean table output with sorting
- Comprehensive error handling
- Modern Node.js best practices

```bash
# Suppress error messages
checkssl -d invalid -s
```

## Exit Codes

The script will exit with a code of 0 if it runs successfully, or 1 if it encounters an error.

## Dependencies

This script uses the built-in `https`, `fs`, `os`, and `path` modules in Node.js, so no additional installation is required.
