/**
 * Helper functions
 */

/**
 * @constant {RegExp} domainRegex
 * @description Regular expression to validate a domain
 */

const domainRegex = /^[a-z0-9.-]+\.[a-z]{2,}$/;
/**
 *
 * @param {string} domain
 * @returns {boolean}
 * @description Check if a domain is valid
 * @example isValidDomain('example.com') => true
 * @example isValidDomain('example') => false
 */
const isValidDomain = (domain) => domainRegex.test(domain);

/**
 * @param {string} domain
 * @param {Function} callback
 * @returns {boolean}
 * @description Validate a domain and call the callback with an error message if the domain is invalid
 * @example validateDomain('example.com', console.log) returns true and logs nothing
 * @example validateDomain('example', console.log) returns false and logs 'Invalid domain: example'
 */
const validateDomain = (domain, callback) => {
  const valid = isValidDomain(domain);
  if (!valid) {
    callback(`Invalid domain: ${domain}`);
    return false;
  }
  return valid;
};

/**
 * @param {number} acc
 * @param {string} domain
 * @returns {number}
 * @description Reduce function to find the length of the longest domain
 * @example ['example.com', 'example.net'].reduce(domainLengthReducer, 0) => 11
 */

const domainLengthReducer = (acc, domain) => Math.max(acc, domain.length);

/**
 * @param {string} date
 * @returns {Date}
 * @description Parse a date in the format dd.mm.yyyy
 * @example parseDate('01.01.2021') => new Date(2021, 0, 1)
 */

const parseDate = (date) => {
  const [day, month, year] = date.split('.');
  return new Date(year, month - 1, day);
};

/**
 * @param {[{domain: string, result: string}]} results
 * @param {"asc"|"desc"}
 * @returns {Array}
 * @description Sort an array of results by the valid_to date
 * @example sortResults([{ result: { valid_to: '01.01.2021' } }, { result: { valid_to: '01.01.2020' } }]) => [{ result: { valid_to: '01.01.2020' } }, { result: { valid_to: '01.01.2021' } }]
 * @example sortResults([{ result: { valid_to: '01.01.2021' } }, { result: { valid_to: '01.01.2020' } }], 'desc') => [{ result: { valid_to: '01.01.2021' } }, { result: { valid_to: '01.01.2020' } }]
 */

const sortResults = (results, direction = 'asc') =>
  results.sort((a, b) => {
    const dateA = parseDate(a.result);
    const dateB = parseDate(b.result);
    if (direction === 'desc') {
      return dateB - dateA;
    }
    return dateA - dateB;
  });

/**
 * @param {[{domain: string, result: string}]} results
 * @param {number} length
 * @returns {Array}
 * @description Format an array of results for printing
 * @example formatResults([{ domain: 'example.com', result: '01.01.2021' }, { domain: 'example.net', result: '01.01.2020' }], 11) => ['| example.com | 01.01.2021 |', '| example.net | 01.01.2020 |']
 */

const formatResults = (results, length) =>
  results.map(
    ({ domain, result }) => `| ${domain.padEnd(length)} | ${result} |`
  );

/**
 * @param {number} length
 * @returns {string}
 * @description Create a separator string
 * @example separator(11) => '============='
 */
const separator = (length) => '='.repeat(length + 17);

/**
 * @returns {void}
 * @description Print the help message
 * @example sendHelp()
 * // Usage: node index.js -d example.com -f file.txt
 * // Example: node index.js -d google.com
 */

const sendHelp = () => {
  console.log('Usage: node index.js -d example.com -f file.txt');
  console.log('Example: node index.js -d google.com\n');
};

/**
 * @param {[string]} results
 * @param {string} line
 * @returns {void}
 * @description Print a table with the results
 * @example printTable(['| example.com | 01.01.2021 |', '| example.net | 01.01.2020 |'], '============================')
 * // ============================
 * // | example.com | 01.01.2021 |
 * // | example.net | 01.01.2020 |
 * // ============================
 */

const printTable = (results, line) => {
  console.log(`${line}\n${results.join('\n')}\n${line}`);
};

/**
 * @returns {void}
 * @description Print an info message
 * @example printInfo()
 * // Please provide a domain or a file with domains
 */

const printInfo = () =>
  console.log('\nPlease provide a domain or a file with domains');

/**
 * @param {[string]} errors
 * @returns {void}
 * @description Print a list of errors
 * @example printErrors(['Invalid domain: example'])
 * // Errors:
 * // Invalid domain: example
 */

const printErrors = (errors) => {
  console.log(`\nErrors:\n\n${errors.join('\n')}`);
};

module.exports = {
  domainLengthReducer,
  domainRegex,
  formatResults,
  printInfo,
  printErrors,
  isValidDomain,
  sendHelp,
  separator,
  sortResults,
  validateDomain,
  printTable,
};
