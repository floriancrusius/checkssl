'use strict';

/**
 * Helper functions for SSL certificate checking
 */

/**
 * Regular expression to validate a domain
 * @constant {RegExp}
 */
const DOMAIN_REGEX = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$/i;

/**
 * Constants for date formatting and validation
 */
const DATE_LOCALE = 'de-DE';
const DATE_FORMAT_OPTIONS = {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
};

/**
 * Check if a domain is valid according to RFC standards
 * @param {string} domain - The domain to validate
 * @returns {boolean} - True if domain is valid, false otherwise
 * @example
 * isValidDomain('example.com'); // true
 * isValidDomain('example'); // false
 * isValidDomain('sub.example.com'); // true
 */
const isValidDomain = (domain) => {
  if (!domain || typeof domain !== 'string') {
    return false;
  }

  const trimmedDomain = domain.trim().toLowerCase();

  // Check length constraints
  if (trimmedDomain.length > 253 || trimmedDomain.length < 1) {
    return false;
  }

  // Check if it matches the regex pattern
  return DOMAIN_REGEX.test(trimmedDomain);
};

/**
 * Validate a domain and call the callback with an error message if invalid
 * @param {string} domain - The domain to validate
 * @param {Function} errorCallback - Callback function to handle errors
 * @returns {boolean} - True if domain is valid, false otherwise
 * @example
 * validateDomain('example.com', console.error); // returns true
 * validateDomain('invalid', console.error); // returns false and logs error
 */
const validateDomain = (domain, errorCallback) => {
  const isValid = isValidDomain(domain);

  if (!isValid && typeof errorCallback === 'function') {
    errorCallback(`Invalid domain format: ${domain}`);
  }

  return isValid;
};

/**
 * Reducer function to find the length of the longest domain
 * @param {number} maxLength - Current maximum length
 * @param {string} domain - Domain to check
 * @returns {number} - Maximum length between current max and domain length
 * @example
 * ['example.com', 'subdomain.example.org'].reduce(domainLengthReducer, 0); // 22
 */
const domainLengthReducer = (maxLength, domain) => {
  if (typeof domain !== 'string') {
    return maxLength;
  }
  return Math.max(maxLength, domain.length);
};

/**
 * Parse a date in German format (dd.mm.yyyy)
 * @param {string} dateString - Date string in format dd.mm.yyyy
 * @returns {Date} - Parsed Date object
 * @throws {Error} - When date string is invalid
 * @example
 * parseDate('01.01.2025'); // Date object for January 1, 2025
 */
const parseDate = (dateString) => {
  if (!dateString || typeof dateString !== 'string') {
    throw new Error('Invalid date string provided');
  }

  const parts = dateString.split('.');
  if (parts.length !== 3) {
    throw new Error(`Invalid date format: ${dateString}. Expected dd.mm.yyyy`);
  }

  const [day, month, year] = parts.map(Number);

  // Validate date components
  if (isNaN(day) || isNaN(month) || isNaN(year)) {
    throw new Error(`Invalid date components in: ${dateString}`);
  }

  if (day < 1 || day > 31 || month < 1 || month > 12 || year < 1000) {
    throw new Error(`Date out of range: ${dateString}`);
  }

  return new Date(year, month - 1, day);
};

/**
 * Sort results by expiration date
 * @param {Array<{domain: string, result: string}>} results - Array of domain results
 * @param {'asc'|'desc'} direction - Sort direction (default: 'asc')
 * @returns {Array} - Sorted array of results
 * @example
 * const results = [
 *   { domain: 'example.com', result: '01.01.2026' },
 *   { domain: 'test.com', result: '01.01.2025' }
 * ];
 * sortResults(results); // Sorted by date ascending
 */
const sortResults = (results, direction = 'asc') => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  return results.sort((a, b) => {
    // Handle explicit error results (containing 'Error')
    if (a.result.includes('Error') && b.result.includes('Error')) {
      return 0;
    }
    if (a.result.includes('Error')) {
      return 1; // Errors go to the end
    }
    if (b.result.includes('Error')) {
      return -1; // Errors go to the end
    }

    try {
      // Try to parse both dates
      let dateA, dateB;
      let aIsValid = false,
        bIsValid = false;

      try {
        dateA = parseDate(a.result);
        aIsValid = true;
      } catch {
        // Date A is invalid
      }

      try {
        dateB = parseDate(b.result);
        bIsValid = true;
      } catch {
        // Date B is invalid
      }

      // If both are invalid, maintain original order
      if (!aIsValid && !bIsValid) {
        return 0;
      }
      // If only A is invalid, put it at the end
      if (!aIsValid) {
        return 1;
      }
      // If only B is invalid, put it at the end
      if (!bIsValid) {
        return -1;
      }

      // Both are valid, sort by date
      return direction === 'desc' ? dateB - dateA : dateA - dateB;
    } catch {
      // Fallback: maintain original order
      return 0;
    }
  });
};

/**
 * Format results for table display
 * @param {Array<{domain: string, result: string}>} results - Array of results to format
 * @param {number} maxDomainLength - Maximum domain length for padding
 * @returns {Array<string>} - Array of formatted strings for display
 * @example
 * formatResults([{ domain: 'example.com', result: '01.01.2025' }], 15);
 * // ['| example.com     | 01.01.2025 |']
 */
const formatResults = (results, maxDomainLength) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const minPadding = Math.max(maxDomainLength, 10); // Minimum 10 characters

  return results.map(({ domain, result }) => {
    const paddedDomain = String(domain).padEnd(minPadding);
    return `| ${paddedDomain} | ${result} |`;
  });
};

/**
 * Create a separator line for table display
 * @param {number} maxDomainLength - Maximum domain length
 * @returns {string} - Separator string
 * @example
 * separator(15); // '================================'
 */
const separator = (maxDomainLength) => {
  const minLength = Math.max(maxDomainLength, 10);
  return '='.repeat(minLength + 17); // 17 for padding and borders
};

/**
 * Display help message to the user
 * @example
 * sendHelp();
 * // Outputs usage instructions
 */
const sendHelp = () => {
  console.log('Usage: checkssl [options]');
  console.log('');
  console.log('Options:');
  console.log('  -d <domain>    Check a specific domain');
  console.log('  -f <file>      Read domains from a file');
  console.log('  -s             Suppress error messages');
  console.log('  -h             Show this help message');
  console.log('  -v, --version  Show version information');
  console.log('');
  console.log('Examples:');
  console.log('  checkssl -d google.com');
  console.log('  checkssl -f domains.txt');
  console.log('  checkssl -d example.com -d another.com');
  console.log('  checkssl --version');
  console.log('');
  console.log('If no options are provided, checkssl will look for ~/.checkssl');
};

/**
 * Print results in a formatted table
 * @param {Array<string>} formattedResults - Pre-formatted result strings
 * @param {string} separatorLine - Separator line for the table
 * @example
 * printTable(['| example.com | 01.01.2025 |'], '====================');
 */
const printTable = (formattedResults, separatorLine) => {
  if (!Array.isArray(formattedResults)) {
    console.error('Error: Invalid results format');
    return;
  }

  console.log(separatorLine);
  formattedResults.forEach((line) => console.log(line));
  console.log(separatorLine);
};

/**
 * Display informational message
 * @example
 * printInfo();
 * // Outputs instructions for providing domains
 */
const printInfo = () => {
  console.log('');
  console.log(
    '💡 Tip: Provide domains using -d option or create ~/.checkssl file',
  );
  console.log('   Example: checkssl -d example.com');
};

/**
 * Display error messages
 * @param {Array<string>} errors - Array of error messages
 * @example
 * printErrors(['Invalid domain: test', 'Connection failed: example.invalid']);
 */
const printErrors = (errors) => {
  if (!Array.isArray(errors) || errors.length === 0) {
    return;
  }

  console.log('');
  console.log('❌ Errors encountered:');
  console.log('');
  errors.forEach((error) => console.log(`   ${error}`));
};

module.exports = {
  // Core validation functions
  isValidDomain,
  validateDomain,

  // Data processing functions
  domainLengthReducer,
  parseDate,
  sortResults,
  formatResults,
  separator,

  // Display functions
  sendHelp,
  printTable,
  printInfo,
  printErrors,

  // Constants (for testing)
  DOMAIN_REGEX,
  DATE_LOCALE,
  DATE_FORMAT_OPTIONS,
};
