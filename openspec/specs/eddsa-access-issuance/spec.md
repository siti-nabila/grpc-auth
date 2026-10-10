# EdDSA Access Issuance

## Purpose

Menetapkan access credential producer yang kompatibel dengan authoritative session verifier, termasuk signing configuration, claims, lifetime dan coordinated security cutover.

## Requirements

### Requirement: Validated Ed25519 signing configuration

The issuer SHALL load configured Ed25519 signing and verification material once, require a matching valid pair and nonempty exact ApplicationName issuer, and fail startup when required configuration is absent, malformed or inconsistent. It SHALL NOT default secrets or reuse service credentials/HMAC secrets as signing material. Secret material SHALL NOT be emitted in logs, response or evidence. EdDSA SHALL be the only access signing algorithm for this producer.

#### Scenario: Valid key pair
- **WHEN** startup receives a valid matching Ed25519 pair and issuer configuration
- **THEN** the issuer can sign credentials accepted by the foundation verifier without per-request key loading

#### Scenario: Invalid or mismatched configuration
- **WHEN** signing configuration is missing, wrong type, malformed, mismatched or has an empty issuer
- **THEN** startup fails safely without serving the configured producer or leaking key material

### Requirement: Session-bound access claims and absolute lifetime

Issued access JWT SHALL contain canonical positive account `sub`, UUID `sid/jti`, exact `iss`, `iat/exp` and authoritative positive `auth_version`. Access expiry SHALL be no later than fifteen minutes from issuance and no later than absolute session expiry. Session and initial refresh SHALL expire seven days from Login without sliding extension. Response expiry fields SHALL match claims/persisted state. Audience SHALL deliberately remain unrequired and unvalidated; verifier skew of thirty seconds SHALL NOT extend the authoritative absolute session deadline.

#### Scenario: Production producer interoperability
- **WHEN** real Login commits and returns its EdDSA access credential
- **THEN** the foundation verifier and session validator accept matching claims/state before their expiry, using the production issuer rather than a synthetic credential fixture

#### Scenario: Expiry boundaries
- **WHEN** a controlled clock reaches the absolute session deadline, including the JWT skew interval
- **THEN** session validation rejects access, and neither JWT tolerance nor refresh state extends that deadline

#### Scenario: Issuer and audience behavior
- **WHEN** a signed credential has the configured issuer and otherwise valid claims with no audience, or an arbitrary audience
- **THEN** audience does not prevent acceptance, while wrong issuer or wrong signature is rejected

### Requirement: Coordinated issuer and consumer activation

Production activation SHALL switch real Login and all registered protected consumers/guards together after foundation readiness gates. The activated pipeline SHALL reject HS256 and sid-less credentials and require clients to Login again. It SHALL NOT provide dual-algorithm fallback, synthetic sessions, permission bypass or partial deployment. Component interoperability evidence SHALL be distinguished from operational key/client/database readiness and full Phase 3 completion.

#### Scenario: Compatible component before activation
- **WHEN** producer components are verified while whole-server prerequisites remain unavailable
- **THEN** component evidence is recorded separately and activation remains blocked rather than partially replacing the registered pipeline

#### Scenario: Coordinated cutover
- **WHEN** foundation activates the compatible issuer and complete protected pipeline after readiness checks
- **THEN** new Login credentials work with session-aware consumers and legacy HS256/sid-less credentials are rejected

Acceptance: [LSP-03/08/09 ledger](../../changes/archive/2026-10-10-auth-login-session-producer/design.md#acceptance-ledger). Foundation owns final server activation and external readiness; this producer supplies compatible components and evidence.
