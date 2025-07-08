const https = require('https');

const getCertificate = (domain) => {
  const options = {
    host: domain,
    port: 443,
    method: 'GET',
  };
  return new Promise((resolve, reject) => {
    const req = https.request(options, function (res) {
      const certificate = res.socket.getPeerCertificate();
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
    req.on('error', (error) => {
      reject(error);
    });
    req.end();
  });
};

module.exports = getCertificate;
