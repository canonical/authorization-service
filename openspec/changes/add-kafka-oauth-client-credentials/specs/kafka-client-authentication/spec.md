## ADDED Requirements

### Requirement: Kafka TLS Configuration
The Kafka integration subsystem SHALL support transport layer security (TLS) for all Kafka broker connections (consumer and admin provisioning), supporting custom CA certificates and optional certificate verification bypass.

#### Scenario: Connecting with custom CA certificate
- **WHEN** Kafka TLS is enabled with a custom CA certificate string or file path
- **THEN** broker connections are established using TLS validated against the provided CA certificate pool

#### Scenario: Connecting with insecure skip verify
- **WHEN** Kafka TLS is enabled with insecure skip verify set to true
- **THEN** broker connections skip server certificate verification

#### Scenario: Default unencrypted transport when TLS is disabled
- **WHEN** Kafka TLS is not enabled and no TLS certificates are configured
- **THEN** broker connections use unencrypted plaintext transport

### Requirement: Kafka SASL/OAUTHBEARER Authentication
The Kafka integration subsystem SHALL implement an RFC 7628 compliant `OAUTHBEARER` SASL mechanism using OAuth2 Client Credentials, fetching access tokens from a configured token endpoint and handling token refresh automatically across connections.

#### Scenario: Successful SASL/OAUTHBEARER handshake
- **WHEN** Kafka OAuth authentication is configured with valid client ID, client secret, and token endpoint URI
- **THEN** the client acquires an access token via client credentials grant, sends the RFC 7628 formatted initial response `n,,\x01auth=Bearer <token>\x01\x01`, and successfully completes the SASL handshake with the broker

#### Scenario: Token endpoint request uses custom CA
- **WHEN** a custom CA is configured for Kafka and OAuth authentication is enabled
- **THEN** the HTTP client communicating with the OAuth token endpoint trusts the custom CA certificate

#### Scenario: Automatic token refresh on broker reconnect
- **WHEN** a Kafka connection is closed and re-established after the previous access token has expired
- **THEN** the token source automatically fetches a new valid access token for the re-dialing SASL handshake

#### Scenario: Broker rejects invalid bearer token
- **WHEN** the Kafka broker returns an error challenge during the SASL exchange
- **THEN** the SASL mechanism completes the error exchange and returns an error containing the broker's error message

#### Scenario: Token retrieval failure
- **WHEN** the OAuth token endpoint is unreachable or credentials are invalid
- **THEN** the connection attempt fails immediately with an error detailing the token acquisition failure

### Requirement: Authenticated Topic Administration
The Kafka topic provisioning subsystem (`EnsureTopics`) SHALL use the TLS and SASL/OAUTHBEARER dialer to perform admin operations against secured brokers.

#### Scenario: Topic provisioning on secured Kafka cluster
- **WHEN** `ensure_topics` runs against a Kafka cluster requiring TLS and OAuth authentication
- **THEN** topic discovery and creation operations successfully dial the cluster using configured TLS and OAuth credentials

#### Scenario: Topic provisioning on unauthenticated Kafka cluster
- **WHEN** `ensure_topics` runs without TLS or OAuth credentials
- **THEN** topic discovery and creation operations connect using unauthenticated plaintext transport

### Requirement: Backward Compatible Unauthenticated Operation
The Kafka integration subsystem SHALL default to unauthenticated plaintext communication when no TLS or OAuth configuration is specified, preserving compatibility with local development and existing integration tests.

#### Scenario: Starting listener without Kafka security configuration
- **WHEN** the `listen` command is executed with broker address and topic configuration but without TLS or OAuth credentials
- **THEN** consumer reader establishes unauthenticated plaintext connections without error
