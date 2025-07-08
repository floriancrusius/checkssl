'use strict';

const https = require('https');

/**
 * Get SSL certificate expiration date for a domain
 * @param {string} domain - The domain to check
 * @param {number} [timeout=5000] - Request timeout in milliseconds
 * @returns {Promise<string>} - Promise that resolves to the formatted expiration date
 * @throws {Error} - When domain is invalid or request fails
 * @example
 * getCertificate('google.com')
 *   .then(date => console.log(`Expires: ${date}`))
 *   .catch(error => console.error(`Error: ${error.message}`));
 */
const getCertificate = (domain, timeout = 5000) => {
  return new Promise((resolve, reject) => {
    // Input validation
    if (!domain || typeof domain !== 'string' || domain.trim().length === 0) {
      return reject(new Error('Domain must be a non-empty string'));
    }

    const options = {
      host: domain,
      port: 443,
      method: 'GET',
      timeout,
      // Security: Reject unauthorized certificates for validation
      rejectUnauthorized: true,
    };

    const req = https.request(options, (res) => {
      try {
        const certificate = res.socket.getPeerCertificate();

        if (!certificate || !certificate.valid_to) {
          return reject(new Error(`No valid certificate found for ${domain}`));
        }

        const expirationDate = new Date(certificate.valid_to);

        // Check if the date is valid
        if (isNaN(expirationDate.getTime())) {
          throw new Error(
            `Invalid certificate date format: ${certificate.valid_to}`,
          );
        }

        // Check if certificate is already expired
        if (expirationDate < new Date()) {
          return reject(
            new Error(`Certificate for ${domain} has already expired`),
          );
        }

        const formattedDate = expirationDate.toLocaleDateString('de-DE', {
          day: '2-digit',
          month: '2-digit',
          year: 'numeric',
        });

        resolve(formattedDate);
      } catch (error) {
        reject(
          new Error(
            `Failed to process certificate for ${domain}: ${error.message}`,
          ),
        );
      }
    });

    // Handle request errors
    req.on('error', (error) => {
      reject(new Error(`Connection failed for ${domain}: ${error.message}`));
    });

    // Handle timeout
    req.on('timeout', () => {
      req.destroy();
      reject(new Error(`Request timeout for ${domain}`));
    });

    // Handle socket errors
    req.on('socket', (socket) => {
      socket.on('secureConnect', () => {
        // Connection established successfully
      });

      socket.on('error', (error) => {
        reject(new Error(`Socket error for ${domain}: ${error.message}`));
      });
    });

    req.end();
  });
};

module.exports = getCertificate;
