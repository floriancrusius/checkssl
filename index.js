#!/usr/bin/env node
'use strict';

const https = require('https');
const fs = require('fs');
const { exit } = require('process');
const { homedir } = require('os');
const { resolve } = require('path');

const getCertificate = async (domain) => {
  const options = {
    host: domain,
    port: 443,
    method: 'GET',
  };
  return new Promise((resolve, reject) => {
    const req = https.request(options, function (res) {
      const certificate = res.connection.getPeerCertificate();
      if (certificate) {
        const notAfter = new Date(certificate.valid_to).toLocaleDateString(
          'de-DE',
          {
            day: '2-digit',
            month: '2-digit',
            year: 'numeric',
          }
        );
        resolve(notAfter);
      }
    });
    req.end();
  });
};
let supressErrorMessages = false;
const input = process.argv.slice(2);
const errors = [];
const domains = input.reduce((acc, curr, i) => {
  if (curr === '-f') {
    const path = resolve(input[i + 1]);
    if (!fs.existsSync(path)) {
      errors.push(`File ${path} does not exist`);
    } else {
      return [
        ...acc,
        ...fs
          .readFileSync(path, 'utf8')
          .split('\n')
          .filter((d) => !!d)
          .filter((domain) => {
            const isValidDomain = /^[a-z0-9.-]+\.[a-z]{2,}$/.test(domain);
            if (!isValidDomain) {
              errors.push(`Invalid domain: ${domain}`);
            }
            return isValidDomain;
          }),
      ];
    }
  } else if (curr === '-d') {
    const domain = input[i + 1];
    const isValidDomain = /^[a-z0-9.-]+\.[a-z]{2,}$/.test(domain);
    if (!acc.includes(domain) && isValidDomain) {
      return [...acc, domain];
    } else {
      errors.push(`Invalid domain: ${domain}`);
    }
  } else if (curr === '-s') {
    supressErrorMessages = true;
  } else if (curr === '-h') {
    console.log('Usage: node index.js -d example.com -f file.txt');
    console.log('Example: node index.js -d google.com\n');
    exit(0);
  }
  return acc;
}, []);
if (domains.length === 0) {
  const path = resolve(homedir(), '.checkssl');

  if (fs.existsSync(path)) {
    domains.push(
      ...fs
        .readFileSync(path, 'utf8')
        .split('\n')
        .filter((d) => !!d)
        .filter((domain) => {
          const isValidDomain = /^[a-z0-9.-]+\.[a-z]{2,}$/.test(domain);
          if (!isValidDomain) {
            errors.push(`Invalid domain: ${domain}`);
          }
          return isValidDomain;
        })
    );
  }
}
const run = async () => {
  const checkDomains = domains.length > 0 ? domains : ['google.com'];
  const maxDomainLength = checkDomains.reduce(
    (acc, domain) => Math.max(acc, domain.length),
    0
  );
  //   const results = await Promise.all(
  //     checkDomains.map(async (domain) => {
  //       const result = await getCertificate(domain);
  //       return `| ${domain.padEnd(maxDomainLength)} | ${result} |`;
  //     })
  //   );
  const results = await Promise.all(
    checkDomains.map(async (domain) => {
      const result = await getCertificate(domain);
      return { domain, result };
    })
  );

  const parseDate = (date) => {
    const [day, month, year] = date.split('.');
    return new Date(year, month - 1, day);
  };

  const sortedResults = results.sort((a, b) => {
    const dateA = parseDate(a.result);
    const dateB = parseDate(b.result);
    return dateA - dateB;
  });

  const formattedResults = sortedResults.map(
    ({ domain, result }) => `| ${domain.padEnd(maxDomainLength)} | ${result} |`
  );
  const line = '='.repeat(maxDomainLength + 17);
  if (domains.length === 0) {
    console.log('Usage: node index.js -d example.com -f file.txt');
    console.log('Example: node index.js -d google.com\n');
  }
  //   console.log(`${line}\n${results.join('\n')}\n${line}`);
  console.log(`${line}\n${formattedResults.join('\n')}\n${line}`);
  if (errors.length > 0 && !supressErrorMessages) {
    console.log(`\nErrors:\n\n${errors.join('\n')}`);
  }
  if (domains.length === 0) {
    console.log('\nPlease provide a domain or a file with domains');
  }
};

run()
  .then(() => exit(0))
  .catch(() => exit(1));
