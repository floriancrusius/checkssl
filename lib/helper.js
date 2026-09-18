'use strict';

/**
 * Helper functions for SSL certificate checking
 */

const DOMAIN_REGEX = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$/i;

const DATE_LOCALE = 'de-DE';
const DATE_FORMAT_OPTIONS = {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
};

const ERROR_LABEL = '   Error  ';

/**
 * @typedef {Object} CheckResult
 * @property {string} domain
 * @property {Date|null} expiresAt  Date the certificate expires, or null on error
 * @property {'valid'|'expiring_soon'|'expired'|'invalid'|'error'} [status]
 * @property {string|null} [authorizationError]
 * @property {string|null} [error]
 */

const isValidDomain = (domain) => {
  if (!domain || typeof domain !== 'string') {
    return false;
  }

  const trimmedDomain = domain.trim().toLowerCase();

  if (trimmedDomain.length > 253 || trimmedDomain.length < 1) {
    return false;
  }

  return DOMAIN_REGEX.test(trimmedDomain);
};

const validateDomain = (domain, errorCallback) => {
  const isValid = isValidDomain(domain);
  if (!isValid && typeof errorCallback === 'function') {
    errorCallback(`Invalid domain format: ${domain}`);
  }
  return isValid;
};

const domainLengthReducer = (maxLength, domain) => {
  if (typeof domain !== 'string') {
    return maxLength;
  }
  return Math.max(maxLength, domain.length);
};

const isDate = (value) =>
  value instanceof Date && !Number.isNaN(value.getTime());

const formatDate = (date) =>
  isDate(date) ? date.toLocaleDateString(DATE_LOCALE, DATE_FORMAT_OPTIONS) : '';

/**
 * Sort results by expiration date. Errors (no expiresAt) always go to the end
 * regardless of direction.
 * @param {CheckResult[]} results
 * @param {'asc'|'desc'} [direction='asc']
 */
const sortResults = (results, direction = 'asc') => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const sign = direction === 'desc' ? -1 : 1;

  return [...results].sort((a, b) => {
    const aHas = isDate(a.expiresAt);
    const bHas = isDate(b.expiresAt);
    if (!aHas && !bHas) return 0;
    if (!aHas) return 1;
    if (!bHas) return -1;
    return sign * (a.expiresAt.getTime() - b.expiresAt.getTime());
  });
};

/**
 * Format results for table display.
 * @param {CheckResult[]} results
 * @param {number} maxDomainLength
 * @returns {string[]}
 */
const formatResults = (results, maxDomainLength) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const minPadding = Math.max(maxDomainLength, 10);

  return results.map(({ domain, expiresAt }) => {
    const paddedDomain = String(domain).padEnd(minPadding);
    const value = isDate(expiresAt) ? formatDate(expiresAt) : ERROR_LABEL;
    return `| ${paddedDomain} | ${value} |`;
  });
};

const csvEscape = (value) => `"${String(value).replace(/"/g, '""')}"`;

/**
 * @param {CheckResult[]} results
 * @returns {string}
 */
const formatResultsCSV = (results) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const header = 'Domain,Expiration';
  const rows = results.map(({ domain, expiresAt }) => {
    const value = isDate(expiresAt) ? formatDate(expiresAt) : 'Error';
    return `${csvEscape(domain)},${csvEscape(value)}`;
  });

  return [header, ...rows].join('\n');
};

/**
 * @param {CheckResult[]} results
 * @returns {string}
 */
const formatResultsJSON = (results) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const jsonResults = results.map(
    ({ domain, expiresAt, status, error, authorizationError }) => {
      const record = {
        domain: String(domain),
        expiration: isDate(expiresAt) ? formatDate(expiresAt) : 'Error',
      };
      if (status) record.status = status;
      if (authorizationError) record.authorizationError = authorizationError;
      if (error) record.error = error;
      return record;
    },
  );

  return JSON.stringify(jsonResults, null, 2);
};

const separator = (maxDomainLength) => {
  const minLength = Math.max(maxDomainLength, 10);
  return '='.repeat(minLength + 17);
};

const sendHelp = () => {
  console.log('Usage: checkssl [options]');
  console.log('');
  console.log('Options:');
  console.log('  -d, --domain <domain>  Check a specific domain');
  console.log('  -f, --file <file>      Read domains from a file');
  console.log('  -s                     Suppress error messages');
  console.log(
    '  --format <type>        Output format: table (default), csv, json',
  );
  console.log('  -h, --help             Show this help message');
  console.log('  -v, --version          Show version information');
  console.log('');
  console.log('Examples:');
  console.log('  checkssl -d google.com');
  console.log('  checkssl -f domains.txt');
  console.log('  checkssl -d example.com -d another.com');
  console.log('  checkssl -d google.com --format csv');
  console.log('  checkssl -f domains.txt --format json');
  console.log('  checkssl --version');
  console.log('');
  console.log('If no options are provided, checkssl will look for ~/.checkssl');
};

const printTable = (formattedResults, separatorLine) => {
  if (!Array.isArray(formattedResults)) {
    console.error('Error: Invalid results format');
    return;
  }

  console.log(separatorLine);
  formattedResults.forEach((line) => console.log(line));
  console.log(separatorLine);
};

const printInfo = () => {
  console.log('');
  console.log(
    '💡 Tip: Provide domains using -d option or create ~/.checkssl file',
  );
  console.log('   Example: checkssl -d example.com');
};

const printErrors = (errors) => {
  if (!Array.isArray(errors) || errors.length === 0) {
    return;
  }

  console.error('');
  console.error('❌ Errors encountered:');
  console.error('');
  errors.forEach((error) => console.error(`   ${error}`));
};

module.exports = {
  isValidDomain,
  validateDomain,
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
  DOMAIN_REGEX,
  DATE_LOCALE,
  DATE_FORMAT_OPTIONS,
};
