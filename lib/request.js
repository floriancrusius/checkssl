'use strict';

const tls = require('tls');

const DEFAULT_TIMEOUT = 5000;
const DEFAULT_PORT = 443;

/**
 * Fetch the peer certificate for a domain via a bare TLS handshake.
 *
 * Does NOT reject when the certificate is expired, self-signed, or has a
 * hostname mismatch — those states are reported through `authorized` and
 * `authorizationError` on the returned object so the caller can classify
 * them. Rejections are reserved for cases where no certificate was obtained
 * at all (network error, timeout, unparseable date).
 *
 * @param {string} domain
 * @param {{ timeout?: number, port?: number }} [options]
 * @returns {Promise<{
 *   validFrom: Date | null,
 *   validTo: Date,
 *   issuer: object | null,
 *   subject: object | null,
 *   subjectAltName: string | null,
 *   authorized: boolean,
 *   authorizationError: string | null,
 * }>}
 */
const getCertificate = (domain, options = {}) => {
  const { timeout = DEFAULT_TIMEOUT, port = DEFAULT_PORT } = options;

  return new Promise((resolve, reject) => {
    if (!domain || typeof domain !== 'string' || domain.trim().length === 0) {
      return reject(new Error('Domain must be a non-empty string'));
    }

    const servername = domain.trim();
    let settled = false;

    const done = (fn, value) => {
      if (settled) return;
      settled = true;
      fn(value);
    };

    const socket = tls.connect(
      {
        host: servername,
        port,
        servername,
        // We want to inspect the cert even if it's invalid — classification
        // happens one layer up.
        rejectUnauthorized: false,
        timeout,
      },
      () => {
        try {
          const cert = socket.getPeerCertificate(false);
          socket.end();

          if (!cert || Object.keys(cert).length === 0 || !cert.valid_to) {
            return done(
              reject,
              new Error(`No valid certificate found for ${servername}`),
            );
          }

          const validTo = new Date(cert.valid_to);
          if (Number.isNaN(validTo.getTime())) {
            return done(
              reject,
              new Error(
                `Invalid certificate date for ${servername}: ${cert.valid_to}`,
              ),
            );
          }

          const validFrom = cert.valid_from ? new Date(cert.valid_from) : null;
          done(resolve, {
            validFrom:
              validFrom && !Number.isNaN(validFrom.getTime())
                ? validFrom
                : null,
            validTo,
            issuer: cert.issuer || null,
            subject: cert.subject || null,
            subjectAltName: cert.subjectaltname || null,
            authorized: socket.authorized === true,
            authorizationError: socket.authorized
              ? null
              : socket.authorizationError
                ? String(socket.authorizationError)
                : null,
          });
        } catch (error) {
          done(
            reject,
            new Error(
              `Failed to process certificate for ${servername}: ${error.message}`,
            ),
          );
        }
      },
    );

    socket.on('error', (error) =>
      done(
        reject,
        new Error(`Connection failed for ${servername}: ${error.message}`),
      ),
    );

    socket.on('timeout', () => {
      socket.destroy();
      done(reject, new Error(`Request timeout for ${servername}`));
    });
  });
};

module.exports = getCertificate;
