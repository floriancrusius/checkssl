#!/usr/bin/env node
'use strict';

const fs = require('fs');
const { exit } = require('process');
const { homedir } = require('os');
const { resolve } = require('path');

const {
  validateDomain,
  isValidDomain,
  domainLengthReducer,
  sortResults,
  formatResults,
  separator,
  sendHelp,
  printTable,
  printInfo,
  printErrors,
} = require('./lib/helper');
const getCertificate = require('./lib/request');

/**
 * Configuration constants
 */
const DEFAULT_CONFIG_FILE = '.checkssl';
const DEFAULT_DOMAIN = 'google.com';
const EXIT_CODES = {
  SUCCESS: 0,
  ERROR: 1,
};

/**
 * Application state
 */
let suppressErrorMessages = false;
const input = process.argv.slice(2);
const errors = [];

/**
 * Add an error to the error collection
 * @param {string} message - Error message to add
 * @returns {boolean} - Always returns false for convenience
 */
const addError = (message) => {
  errors.push(message);
  return false;
};

/**
 * Read domains from a file
 * @param {string} filePath - Path to the file containing domains
 * @returns {Array<string>} - Array of valid domains
 */
const readDomainsFromFile = (filePath) => {
  try {
    if (!fs.existsSync(filePath)) {
      addError(`File ${filePath} does not exist`);
      return [];
    }

    const fileContent = fs.readFileSync(filePath, 'utf8');

    return fileContent
      .split('\n')
      .map((line) => line.split('#')[0].trim()) // Remove comments and whitespace
      .filter((domain) => domain.length > 0) // Remove empty lines
      .filter((domain) => validateDomain(domain, addError)); // Validate domains
  } catch (error) {
    addError(`Failed to read file ${filePath}: ${error.message}`);
    return [];
  }
};

/**
 * Parse command line arguments and extract domains
 * @param {Array<string>} args - Command line arguments
 * @returns {Array<string>} - Array of domains to check
 */
const parseArguments = (args) => {
  const domains = [];

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    switch (arg) {
      case '-f': {
        const filePath = args[i + 1];
        if (!filePath) {
          addError('Missing file path after -f option');
          break;
        }

        const resolvedPath = resolve(filePath);
        const fileDomains = readDomainsFromFile(resolvedPath);
        domains.push(...fileDomains);
        i++; // Skip the next argument (file path)
        break;
      }

      case '-d': {
        const domain = args[i + 1];
        if (!domain) {
          addError('Missing domain after -d option');
          break;
        }

        if (!domains.includes(domain) && isValidDomain(domain)) {
          domains.push(domain);
        } else if (!isValidDomain(domain)) {
          addError(`Invalid domain: ${domain}`);
        }
        i++; // Skip the next argument (domain)
        break;
      }

      case '-s':
        suppressErrorMessages = true;
        break;

      case '-h':
        sendHelp();
        exit(EXIT_CODES.SUCCESS);
      // falls through - unreachable due to exit()

      default:
        // Ignore unknown arguments
        break;
    }
  }

  return domains;
};

/**
 * Load domains from default config file if no domains provided
 * @returns {Array<string>} - Array of domains from config file
 */
const loadDefaultDomains = () => {
  const configPath = resolve(homedir(), DEFAULT_CONFIG_FILE);

  if (fs.existsSync(configPath)) {
    return readDomainsFromFile(configPath);
  }

  return [];
};

/**
 * Check SSL certificates for all domains
 * @param {Array<string>} domainsToCheck - Array of domains to check
 * @returns {Promise<Array<{domain: string, result: string}>>} - Results of certificate checks
 */
const checkCertificates = async (domainsToCheck) => {
  const results = await Promise.all(
    domainsToCheck.map(async (domain) => {
      try {
        const result = await getCertificate(domain);
        return { domain, result };
      } catch (error) {
        const errorMessage = `${domain}: ${error.message}`;
        addError(errorMessage);
        return { domain, result: '   Error  ' };
      }
    }),
  );

  return results;
};

/**
 * Display results in a formatted table
 * @param {Array<{domain: string, result: string}>} results - Certificate check results
 * @param {Array<string>} originalDomains - Original domain list for help display
 */
const displayResults = (results, originalDomains) => {
  if (results.length === 0) {
    sendHelp();
    printInfo();
    return;
  }

  const maxDomainLength = results
    .map(({ domain }) => domain)
    .reduce(domainLengthReducer, 0);

  const sortedResults = sortResults(results);
  const formattedResults = formatResults(sortedResults, maxDomainLength);
  const separatorLine = separator(maxDomainLength);

  // Show help if no domains were originally provided
  if (originalDomains.length === 0) {
    sendHelp();
  }

  printTable(formattedResults, separatorLine);

  // Show errors if not suppressed
  if (errors.length > 0 && !suppressErrorMessages) {
    printErrors(errors);
  }

  // Show info if no domains were originally provided
  if (originalDomains.length === 0) {
    printInfo();
  }
};

/**
 * Main application function
 */
const main = async () => {
  try {
    // Parse command line arguments
    const domains = parseArguments(input);

    // Load default domains if none provided via arguments
    if (
      domains.length === 0 &&
      !input.includes('-d') &&
      !input.includes('-f')
    ) {
      const defaultDomains = loadDefaultDomains();
      domains.push(...defaultDomains);
    }

    // Use fallback domain if still no domains
    const domainsToCheck = domains.length > 0 ? domains : [DEFAULT_DOMAIN];

    // Check certificates
    const results = await checkCertificates(domainsToCheck);

    // Display results
    displayResults(results, domains);
  } catch (error) {
    console.error(`Unexpected error: ${error.message}`);
    exit(EXIT_CODES.ERROR);
  }
};

// Handle unhandled promise rejections
process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled Rejection at:', promise, 'reason:', reason);
  exit(EXIT_CODES.ERROR);
});

// Handle uncaught exceptions
process.on('uncaughtException', (error) => {
  console.error('Uncaught Exception:', error);
  exit(EXIT_CODES.ERROR);
});

// Run the application
if (require.main === module) {
  main()
    .then(() => exit(EXIT_CODES.SUCCESS))
    .catch((error) => {
      console.error(`Application error: ${error.message}`);
      exit(EXIT_CODES.ERROR);
    });
}

module.exports = { main, parseArguments, readDomainsFromFile };
