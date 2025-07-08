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

let supressErrorMessages = false;
const input = process.argv.slice(2);
const errors = [];

const setError = (message) => {
  errors.push(message);
  return false;
};

const domains = input.reduce((acc, curr, i) => {
  switch (curr) {
    case '-f': {
      const path = resolve(input[i + 1]);
      if (!fs.existsSync(path)) {
        errors.push(`File ${path} does not exist`);
      } else {
        return [
          ...acc,
          ...fs
            .readFileSync(path, 'utf8')
            .split('\n')
            .map((d) => d.split('#')[0].trim())
            .filter((d) => !!d)
            .filter((domain) => validateDomain(domain, setError)),
        ];
      }
      break;
    }
    case '-d': {
      const domain = input[i + 1];
      if (!acc.includes(domain) && isValidDomain(domain)) {
        return [...acc, domain];
      } else {
        errors.push(`Invalid domain: ${domain}`);
      }
      break;
    }
    case '-s':
      supressErrorMessages = true;
      break;
    case '-h':
      sendHelp();
      exit(0);
      break;
    default:
      return acc;
  }
  return acc;
}, []);

if (domains.length === 0 && !input.includes('-d') && !input.includes('-f')) {
  const path = resolve(homedir(), '.checkssl');

  if (fs.existsSync(path)) {
    domains.push(
      ...fs
        .readFileSync(path, 'utf8')
        .split('\n')
        .map((d) => d.split('#')[0].trim())
        .filter((d) => !!d)
        .filter((domain) => validateDomain(domain, setError))
    );
  }
}

const run = async () => {
  const checkDomains = domains.length > 0 ? domains : ['google.com'];
  const maxDomainLength = checkDomains.reduce(domainLengthReducer, 0);

  const results = await Promise.all(
    checkDomains.map(async (domain) => {
      try {
        const result = await getCertificate(domain);
        return { domain, result };
      } catch (error) {
        errors.push(`${domain}: ${error.message}`);
        return { domain, result: '   Error  ' };
      }
    })
  );

  const sortedResults = sortResults(results);

  const formattedResults = formatResults(sortedResults, maxDomainLength);
  const line = separator(maxDomainLength);
  if (domains.length === 0) {
    sendHelp();
  }

  printTable(formattedResults, line);

  if (errors.length > 0 && !supressErrorMessages) {
    printErrors(errors);
  }
  if (domains.length === 0) {
    printInfo();
  }
};

run()
  .then(() => exit(0))
  .catch((err) => {
    console.error(err);
    exit(1);
  });
