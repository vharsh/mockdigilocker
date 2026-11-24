# Security policy

Mock DigiLocker is a **testing sandbox**. It must never hold real identity data and is not designed
to be exposed to the public internet.

## Safe deployment defaults

* Local defaults (`admin`/`admin123`, fixed OTP `123456`, persona list on the login screen) are for
  laptops and CI only. For shared QA/UAT: set `ADMIN_PASSWORD`, `SESSION_SECRET`,
  `RSA_PRIVATE_KEY_FILE`, `SECURE_COOKIES=true` and `CORS_ORIGINS`, create per-person console users
  with least-privilege roles, and restrict network access (VPN / IP allow-list).
* Console passwords and persona PINs are hashed with PBKDF2-SHA256 (120k iterations, random salt).
  Console logins are rate-limited. Sessions are server-side revocable.
* Redirect URIs are exact-matched; invalid clients or redirect URIs never redirect (open-redirect
  protection). PKCE (S256) is supported and can be enforced. Authorization codes are single-use, and
  replay revokes the tokens they issued.
* Consent and login pages send `X-Frame-Options: DENY` and `Cache-Control: no-store`.
* The request inspector stores full request/response headers and bodies, including bearer tokens
  and client secrets sent by partner apps. Treat inspector access (any console role) as sensitive
  within your team, and use sandbox-only credentials.

## Data handling

* All personas, documents and identifiers are synthetic. Documents are watermarked
  "DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION". The signup API only accepts synthetic UIDs
  starting with `0000`.
* If real personal data is entered by mistake: delete the persona in the console, run
  *Reset → all*, and purge backups of the database volume.

## Reporting a vulnerability

Please report vulnerabilities privately to the maintainers (GitHub *Security → Report a
vulnerability* on the repository) rather than in a public issue. Include steps to reproduce and the
affected version. We aim to acknowledge within 5 working days.

Out of scope: findings that rely on the documented local defaults (default password, fixed OTP,
persona hints) in a non-hardened local deployment.
