#!/usr/bin/env node
'use strict';

const fs = require('fs');
const { exit } = require('process');
const { homedir } = require('os');
const { resolve } = require('path');
const { parseArgs } = require('util');

const {
  validateDomain,
  isValidDomain,
  domainLengthReducer,
  sortResults,
  formatResults,
  formatResultsCSV,
  formatResultsJSON,
  separator,
  sendHelp,
  printTable,
  printInfo,
  printErrors,
} = require('./lib/helper');
const getCertificate = require('./lib/request');
const packageJson = require('./package.json');

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
let outputFormat = 'table'; // Default format
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

const CLI_OPTIONS = {
  file: { type: 'string', short: 'f', multiple: true },
  domain: { type: 'string', short: 'd', multiple: true },
  format: { type: 'string' },
  silent: { type: 'boolean', short: 's' },
  help: { type: 'boolean', short: 'h' },
  version: { type: 'boolean', short: 'v' },
};

const VALID_FORMATS = ['table', 'csv', 'json'];

/**
 * Parse command line arguments and collect the domains to check.
 * @param {Array<string>} args
 * @returns {Array<string>}
 */
const parseArguments = (args) => {
  let values;
  try {
    ({ values } = parseArgs({
      args,
      options: CLI_OPTIONS,
      strict: true,
      allowPositionals: true,
    }));
  } catch (error) {
    addError(`Syntax error: ${error.message}`);
    sendHelp();
    return [];
  }

  if (values.help) {
    sendHelp();
    exit(EXIT_CODES.SUCCESS);
  }

  if (values.version) {
    console.log(`checkssl v${packageJson.version}`);
    exit(EXIT_CODES.SUCCESS);
  }

  if (values.silent) {
    suppressErrorMessages = true;
  }

  if (values.format !== undefined) {
    if (VALID_FORMATS.includes(values.format)) {
      outputFormat = values.format;
    } else {
      addError(
        `Invalid format: ${values.format}. Supported formats: ${VALID_FORMATS.join(', ')}`,
      );
    }
  }

  const domains = [];
  const addDomain = (domain) => {
    if (!domains.includes(domain)) domains.push(domain);
  };

  for (const filePath of values.file || []) {
    const fileDomains = readDomainsFromFile(resolve(filePath));
    fileDomains.forEach(addDomain);
  }

  for (const domain of values.domain || []) {
    if (isValidDomain(domain)) {
      addDomain(domain);
    } else {
      addError(`Invalid domain: ${domain}`);
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

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;
const DEFAULT_CONCURRENCY = 20;

/**
 * Derive a status label from raw certificate info.
 * @param {{ validTo: Date, authorized: boolean }} info
 * @param {number} nowMs
 * @returns {'valid'|'expiring_soon'|'expired'|'invalid'}
 */
const classifyStatus = (info, nowMs = Date.now()) => {
  const remaining = info.validTo.getTime() - nowMs;
  if (remaining <= 0) return 'expired';
  if (!info.authorized) return 'invalid';
  if (remaining <= THIRTY_DAYS_MS) return 'expiring_soon';
  return 'valid';
};

/**
 * Run an async worker over items with a fixed concurrency limit.
 * Preserves input order in the result.
 * @template T, R
 * @param {T[]} items
 * @param {number} limit
 * @param {(item: T, index: number) => Promise<R>} worker
 * @returns {Promise<R[]>}
 */
const mapWithConcurrency = async (items, limit, worker) => {
  const results = new Array(items.length);
  const width = Math.max(1, Math.min(limit, items.length));
  let cursor = 0;

  const runners = Array.from({ length: width }, async () => {
    while (cursor < items.length) {
      const index = cursor++;
      results[index] = await worker(items[index], index);
    }
  });

  await Promise.all(runners);
  return results;
};

const checkOne = async (domain) => {
  try {
    const info = await getCertificate(domain);
    const status = classifyStatus(info);
    if (status === 'invalid') {
      addError(
        `${domain}: ${info.authorizationError || 'invalid certificate'}`,
      );
    }
    return {
      domain,
      expiresAt: info.validTo,
      status,
      authorizationError: info.authorizationError,
      error: null,
    };
  } catch (error) {
    addError(`${domain}: ${error.message}`);
    return {
      domain,
      expiresAt: null,
      status: 'error',
      authorizationError: null,
      error: error.message,
    };
  }
};

/**
 * Check SSL certificates for all domains, capped at `concurrency` in flight.
 * @param {Array<string>} domainsToCheck
 * @param {number} [concurrency=DEFAULT_CONCURRENCY]
 * @returns {Promise<Array<import('./lib/helper').CheckResult>>}
 */
const checkCertificates = (domainsToCheck, concurrency = DEFAULT_CONCURRENCY) =>
  mapWithConcurrency(domainsToCheck, concurrency, checkOne);

/**
 * Display results in the specified format
 * @param {Array<{domain: string, result: string}>} results - Certificate check results
 * @param {Array<string>} originalDomains - Original domain list for help display
 */
const displayResults = (results, originalDomains) => {
  if (results.length === 0) {
    if (outputFormat === 'table') {
      sendHelp();
      printInfo();
    }
    return;
  }

  const sortedResults = sortResults(results);

  // Output based on selected format
  switch (outputFormat) {
    case 'csv':
      console.log(formatResultsCSV(sortedResults));
      break;

    case 'json':
      console.log(formatResultsJSON(sortedResults));
      break;

    case 'table':
    default: {
      const maxDomainLength = results
        .map(({ domain }) => domain)
        .reduce(domainLengthReducer, 0);

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
      break;
    }
  }

  // For non-table formats, show errors separately if not suppressed
  if (outputFormat !== 'table' && errors.length > 0 && !suppressErrorMessages) {
    console.error('\n--- Errors ---');
    errors.forEach((error) => console.error(error));
  }
};

/**
 * Main application function
 */
const main = async () => {
  try {
    // Parse command line arguments
    const domains = parseArguments(input);

    // Load default domains if the user provided no source flag at all
    const DOMAIN_SOURCE_FLAGS = ['-d', '--domain', '-f', '--file'];
    const userGaveSource = input.some((arg) =>
      DOMAIN_SOURCE_FLAGS.some(
        (flag) => arg === flag || arg.startsWith(`${flag}=`),
      ),
    );
    if (domains.length === 0 && !userGaveSource) {
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
