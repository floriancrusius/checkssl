# checkssl

`checkssl` is a Node.js script that checks the expiration dates of SSL certificates for a list of domains. It can take input from the command line or from a file.

## Installation

At the moment, the script is not published to npm, so
you can install the package globally using npm:

```bash
npm install -g git+ssh://git@github.com:floriancrusius/checkssl.git#v1.0.0

```

or

```bash
npm install -g git+https://github.com/floriancrusius/checkssl.git#v1.0.0

```

## Usage

You can provide domains directly with one or multiple `-d` option, or you can provide one or multiple file containing a list of domains with multiple `-f` option. If no domains or files are provided, the script will look for a file named `.checkssl` in your home directory.

Here are some examples:

```bash
# Check a single domain
checkssl -d example.com

# Check multiple domains
checkssl -d example.com -d example.org

# Check domains from a file
checkssl -f /path/to/your/file/contains/domains
```

example file:

```bash
# `#`comments are allowed
example.com # this is a comment
example.org
```

The script will output a table with the domains and their SSL certificate expiration dates, sorted in ascending order by date.

## Error Handling

If a domain is invalid or a file does not exist, the script will print an error message. You can suppress these error messages with the `-s` option.

```bash
# Suppress error messages
checkssl -d invalid -s
```

## Exit Codes

The script will exit with a code of 0 if it runs successfully, or 1 if it encounters an error.

## Dependencies

This script uses the built-in `https`, `fs`, `os`, and `path` modules in Node.js, so no additional installation is required.
