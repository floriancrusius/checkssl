# checkssl

[![Node.js Version](https://img.shields.io/badge/node-%3E%3D18.0.0-brightgreen.svg)](https://nodejs.org/)
[![License](https://img.shields.io/badge/license-ISC-blue.svg)](LICENSE)

`checkssl` is a modern Node.js CLI tool that checks SSL certificate expiration dates for domains. It provides a clean, tabular output showing when certificates will expire, helping you stay ahead of certificate renewals.

## Features

- ✅ Check single or multiple domains
- 📁 Read domains from files (with comment support)
- 🏠 Auto-load domains from `~/.checkssl` config file
- 📊 Multiple output formats: table, CSV, JSON
- 🔄 Sort results by expiration date
- ⚡ Fast concurrent certificate checking
- 🚫 Error suppression option
- 🛡️ Robust error handling and validation
- 🌍 Support for German (dd.mm.yyyy) and American (mm/dd/yyyy) date formats

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
  -d <domain>      Check a specific domain
  -f <file>        Read domains from a file
  -s               Suppress error messages
  --format <type>  Output format: table (default), csv, json
  -h               Show help message
  -v, --version    Show version information
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

# Export results as CSV
checkssl -d example.com -d google.com --format csv

# Export results as JSON
checkssl -f domains.txt --format json

# Combine options
checkssl -f production-domains.txt -d staging.example.com --format json -s
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

## Output Formats

The tool supports three output formats to suit different use cases:

### Table Format (Default)

The traditional clean table format for human-readable output:

```
===================================
| example.com     | 15.03.2025 |
| google.com      | 22.07.2025 |
| expired.com     |   Error    |
===================================
```

### CSV Format

Perfect for importing into spreadsheets or data processing:

```bash
checkssl -d example.com -d google.com --format csv
```

Output:

```csv
Domain,Expiration
"example.com","15.03.2025"
"google.com","22.07.2025"
```

### JSON Format

Structured data format ideal for APIs and automation:

```bash
checkssl -d example.com -d google.com --format json
```

Output:

```json
[
  {
    "domain": "example.com",
    "expiration": "15.03.2025"
  },
  {
    "domain": "google.com",
    "expiration": "22.07.2025"
  }
]
```

Results are automatically sorted by expiration date (earliest first) in all formats.

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

### v1.1.0 (Latest)

- ✨ **New**: Multiple output formats (table, CSV, JSON)
- ✨ **New**: `--format` option for selecting output format
- ✨ **New**: Support for both German (dd.mm.yyyy) and American (mm/dd/yyyy) date formats
- 🔧 **Improved**: Enhanced help documentation with more examples
- 🔧 **Improved**: Better error handling for structured formats
- 📚 **Docs**: Comprehensive README updates with integration examples
- 🧪 **Tests**: Extensive test coverage for new format features

### v1.0.0

- 🎉 Initial release
- ✅ Support for single and multiple domain checking
- 📁 File-based domain input with comment support
- 📊 Clean table output with sorting
- 🛡️ Comprehensive error handling
- ⚡ Modern Node.js best practices

```bash
# Suppress error messages
checkssl -d invalid -s
```

## Exit Codes

The script will exit with a code of 0 if it runs successfully, or 1 if it encounters an error.

## Dependencies

This script uses the built-in `https`, `fs`, `os`, and `path` modules in Node.js, so no additional installation is required.

## Integration & Automation

### Monitoring Scripts

Use checkssl in monitoring scripts with JSON output for easy parsing:

```bash
#!/bin/bash
# Check critical domains and alert if certificates expire soon

DOMAINS="api.example.com,web.example.com,cdn.example.com"
CRITICAL_DAYS=30

# Get certificate data as JSON
CERT_DATA=$(checkssl -d api.example.com -d web.example.com --format json)

# Process with jq (or your preferred JSON parser)
echo "$CERT_DATA" | jq -r '.[] | select(.expiration != "Error") | "\(.domain): \(.expiration)"'
```

### CSV Integration

Export to CSV for spreadsheet analysis:

```bash
# Export all production domains to CSV
checkssl -f production-domains.txt --format csv > certificates-$(date +%Y%m%d).csv

# Import into Excel, Google Sheets, or other tools
```

### CI/CD Integration

Integrate into your deployment pipeline:

```bash
# GitHub Actions / GitLab CI example
- name: Check SSL Certificates
  run: |
    checkssl -f .github/domains.txt --format json > cert-report.json
    # Parse JSON and fail if certificates expire within 30 days
```

### API Endpoints

Use JSON output to feed certificate data to monitoring systems:

```bash
# Send certificate data to monitoring API
CERT_JSON=$(checkssl -f critical-domains.txt --format json -s)
curl -X POST https://monitoring.example.com/certificates \
  -H "Content-Type: application/json" \
  -d "$CERT_JSON"
```

## Advanced Usage

### Filtering and Processing

Combine with standard Unix tools for powerful filtering:

```bash
# Get only domains expiring in the next 30 days (requires date parsing)
checkssl -f all-domains.txt --format csv | grep -v "Error" | head -10

# Count total domains checked
checkssl -f domains.txt --format json | jq length

# Extract only domain names
checkssl -f domains.txt --format json | jq -r '.[].domain'

# Get domains with errors
checkssl -f domains.txt --format json | jq -r '.[] | select(.expiration == "Error") | .domain'
```

### Batch Processing

Process large domain lists efficiently:

```bash
# Process domains in batches to avoid rate limiting
split -l 50 large-domain-list.txt batch_
for batch in batch_*; do
    checkssl -f "$batch" --format csv >> all-results.csv
    sleep 5  # Rate limiting pause
done
```

### Custom Domain Lists

Create specialized domain lists for different environments:

```bash
# domains/production.txt
api.example.com
web.example.com
cdn.example.com

# domains/staging.txt
api-staging.example.com
web-staging.example.com

# domains/development.txt
api-dev.example.com
web-dev.example.com

# Check all environments
for env in production staging development; do
    echo "=== $env Environment ==="
    checkssl -f "domains/$env.txt" --format table
done
```

## Troubleshooting

### Common Issues

#### Invalid Domain Format

```bash
# ❌ Invalid
checkssl -d "not-a-domain"

# ✅ Valid
checkssl -d "example.com"
```

#### Network Timeouts

```bash
# If you're experiencing timeouts, the tool uses a 5-second timeout by default
# For slow networks, you might see "Error" results for valid domains
```

#### File Not Found

```bash
# Make sure the domain file exists and is readable
ls -la domains.txt
checkssl -f domains.txt
```

#### Permission Issues

```bash
# If you can't access ~/.checkssl
ls -la ~/.checkssl
chmod 644 ~/.checkssl
```

### Debug Mode

For troubleshooting, avoid the `-s` flag to see detailed error messages:

```bash
# See all error details
checkssl -f problematic-domains.txt

# Compare with suppressed errors
checkssl -f problematic-domains.txt -s
```

### Output Validation

Validate your output format:

```bash
# Validate JSON output
checkssl -d google.com --format json | jq .

# Validate CSV output
checkssl -d google.com --format csv | csv-validate
```
