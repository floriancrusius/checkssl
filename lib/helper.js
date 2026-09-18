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
const DAYS_COL_WIDTH = 17;
const MS_PER_DAY = 24 * 60 * 60 * 1000;

const ANSI = {
  red: '\x1b[31m',
  yellow: '\x1b[33m',
  green: '\x1b[32m',
  dim: '\x1b[2m',
  reset: '\x1b[0m',
};

/**
 * Whether to emit ANSI colors. Off when stdout is not a TTY or when
 * NO_COLOR is set (https://no-color.org).
 */
const shouldColor = () =>
  Boolean(process.stdout && process.stdout.isTTY) && !process.env.NO_COLOR;

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

const daysUntil = (date, nowMs) =>
  isDate(date) ? Math.ceil((date.getTime() - nowMs) / MS_PER_DAY) : null;

const formatDays = (days) => {
  if (days === null) return 'N/A';
  if (days < 0) return `expired ${-days}d ago`;
  if (days === 0) return 'today';
  if (days === 1) return 'in 1 day';
  return `in ${days} days`;
};

/**
 * Pick a status when the caller didn't supply one. Cannot distinguish
 * 'invalid' (which needs the authorized flag), only the time-based buckets.
 */
const deriveStatus = (days) => {
  if (days === null) return 'error';
  if (days < 0) return 'expired';
  if (days <= 30) return 'expiring_soon';
  return 'valid';
};

const colorFor = (status) => {
  switch (status) {
    case 'expired':
    case 'invalid':
      return ANSI.red;
    case 'expiring_soon':
      return ANSI.yellow;
    case 'valid':
      return ANSI.green;
    default:
      return ANSI.dim;
  }
};

const wrapColor = (color, text) =>
  shouldColor() ? `${color}${text}${ANSI.reset}` : text;

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
 * Format results for table display, with a coloured "expires in" column.
 * @param {CheckResult[]} results
 * @param {number} maxDomainLength
 * @param {number} [nowMs]
 * @returns {string[]}
 */
const formatResults = (results, maxDomainLength, nowMs = Date.now()) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const minPadding = Math.max(maxDomainLength, 10);

  return results.map(({ domain, expiresAt, status }) => {
    const paddedDomain = String(domain).padEnd(minPadding);
    const dateStr = isDate(expiresAt) ? formatDate(expiresAt) : ERROR_LABEL;
    const days = daysUntil(expiresAt, nowMs);
    const daysStr = formatDays(days).padStart(DAYS_COL_WIDTH);
    const effectiveStatus = status || deriveStatus(days);
    const daysCell = wrapColor(colorFor(effectiveStatus), daysStr);
    return `| ${paddedDomain} | ${dateStr} | ${daysCell} |`;
  });
};

const csvEscape = (value) => `"${String(value).replace(/"/g, '""')}"`;

/**
 * @param {CheckResult[]} results
 * @returns {string}
 */
const formatResultsCSV = (results, nowMs = Date.now()) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const header = 'Domain,Expiration,DaysUntilExpiry';
  const rows = results.map(({ domain, expiresAt }) => {
    const value = isDate(expiresAt) ? formatDate(expiresAt) : 'Error';
    const days = daysUntil(expiresAt, nowMs);
    return `${csvEscape(domain)},${csvEscape(value)},${days === null ? '' : days}`;
  });

  return [header, ...rows].join('\n');
};

/**
 * @param {CheckResult[]} results
 * @returns {string}
 */
const formatResultsJSON = (results, nowMs = Date.now()) => {
  if (!Array.isArray(results)) {
    throw new Error('Results must be an array');
  }

  const jsonResults = results.map(
    ({ domain, expiresAt, status, error, authorizationError }) => {
      const days = daysUntil(expiresAt, nowMs);
      const record = {
        domain: String(domain),
        expiration: isDate(expiresAt) ? formatDate(expiresAt) : 'Error',
        daysUntilExpiry: days,
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
  // `| dom | dd.mm.yyyy | days-col |` — three borders plus the fixed widths.
  return '='.repeat(minLength + DAYS_COL_WIDTH + 20);
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
