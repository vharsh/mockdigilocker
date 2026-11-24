# Test personas

Predefined personas are seeded on first start and restored by **Reset → users**. Every persona's
security PIN is `123456`. With the default `OTP_MODE=fixed`, the OTP is also `123456`. Sign in with
the mobile number, the e-mail or the username.

| # | Name | Mobile | E-mail | Persona key | Documents | Use it to test |
|---|---|---|---|---|---|---|
| 1 | Asha Testkumari | 9999900001 | asha.normal@sandbox.test | `normal_all_docs` | Aadhaar, PAN, Driving Licence (valid); e-Aadhaar | the happy path |
| 2 | Nikhil Nodocs | 9999900002 | nikhil.nodocs@sandbox.test | `no_documents` | none; no e-Aadhaar | empty states, `aadhaar_not_linked` |
| 3 | Priya Partialdas | 9999900003 | priya.partial@sandbox.test | `partial_documents` | Aadhaar, PAN | `req_doctype` asking for a missing DL |
| 4 | Rahul Expiredwala | 9999900004 | rahul.expired@sandbox.test | `expired_documents` | Aadhaar, PAN valid; DL and RC **expired** | expiry handling |
| 5 | Irfan Invalidkhan | 9999900005 | irfan.invalid@sandbox.test | `invalid_documents` | Aadhaar; PAN **invalid**; DL **revoked** | verification-status handling |
| 6 | Meera Multidocs | 9999900006 | meera.multi@sandbox.test | `multiple_same_issuer` | Aadhaar, PAN, DL + **two** RCs from MoRTH | several docs from one issuer |
| 7 | Dev Disabledson | 9999900007 | dev.disabled@sandbox.test | `disabled_account` | Aadhaar, PAN | login failure, disabled-user tokens |
| 8 | Pooja Pullwali | 9999900008 | pooja.pullable@sandbox.test | `pull_documents` | none in the locker; issuer records for PAN `ZZZPW0008W` and DL `KL0020210000008` | Pull Document |

Stable DigiLocker IDs are `a1b2c3d4-000N-4000-8000-00000000000N` (N = persona number), so test
scripts can hard-code them.

## Synthetic identifiers

| Identifier | Format | Why it's safe |
|---|---|---|
| Mobile | `99999xxxxx` | reserved test-like block. Never use real numbers |
| E-mail | `*@sandbox.test` | `.test` is a reserved TLD (RFC 2606) |
| Aadhaar | documents show only `XXXX XXXX nnnn`. Synthetic UID `00000000nnnn` | real Aadhaar numbers never start with 0 or 1 |
| PAN | `ZZZP?nnnn?` | valid PAN *format* so regex checks pass. `ZZZ` prefixes are not used in practice |
| Driving licence / RC | state code + RTO `00` | RTO code 00 is not issued |

Valid formats mean your input validation is exercised realistically. They are fictional values.
Never replace them with real ones.

## What each document contains

Every document has a PDF (watermarked "DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION", with a status
stamp when it is not valid) and a certificate XML. The fields are:

* **Aadhaar (ADHAR):** masked number, name, DOB, gender, care-of, address
* **PAN (PANCR):** PAN, name, father's name, DOB
* **Driving licence (DRVLC):** licence number, name, DOB, S/D/W of, address, classes of vehicle, blood group, issuing authority, validity
* **Vehicle RC (RVCER):** registration number, owner, address, maker/model, fuel, chassis (masked), validity

Editing a persona's profile (portal **Account** page or console) updates the name, DOB, gender and
address on their documents.

## Creating your own

**Console → Test personas → New persona.** Set identifiers, optionally generate Aadhaar/PAN/DL, then
**Assign document** to add more with any status, issue date or expiry (a past `valid_to` means
expired). Document statuses:

| Status | Listing | File / XML | Shown as |
|---|---|---|---|
| `valid` | listed | 200 | VALID |
| `expired` (or `valid_to` in the past) | listed | 200, XML `status="E"`, PDF stamped EXPIRED | EXPIRED |
| `invalid` | listed | 200, XML `status="I"` | INVALID |
| `revoked` | listed | 200, XML `status="R"` | REVOKED |
| `unavailable` | listed | **503 `repository_service_unpublished`** | UNAVAILABLE |

Persona actions in the console: enable/disable, reset sessions (kills portal sessions and OAuth
tokens), restore persona documents, edit, delete.
