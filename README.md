# gRPC Authentication Service

A gRPC-based authentication service built with Go, featuring user registration, login, and JWT token generation. This project demonstrates best practices for building scalable microservices with protocol buffers and PostgreSQL.

## 📋 Features

- **User Registration & Authentication** - Register new users and authenticate with email/password
- **JWT Token Generation** - Secure token-based authentication
- **gRPC Services** - High-performance RPC communication
- **PostgreSQL Integration** - Persistent data storage with connection pooling
- **Structured Logging** - Comprehensive logging with file rotation
- **Error Handling** - Centralized error dictionary with multi-language support
- **Transaction Support** - Database transaction management for data consistency
- **Database Query Logging** - SQL query logging with performance metrics

## 🏗️ Project Structure

```
grpc-auth/
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── configs/                # Configuration management
│   ├── features/               # Business logic features
│   ├── handler/                # gRPC request handlers
│   └── repositories/           # Data access layer
├── pkg/
│   ├── database/               # Database wrapper & utilities
│   ├── helpers/                # gRPC and database error adapters
│   ├── jwt/                    # JWT token utilities
│   ├── logger/                 # Logging configuration
│   └── utils/                  # Utility functions
├── logs/                       # Application logs (auto-generated)
├── env.yaml                    # Environment configuration
├── Makefile                    # Build & development commands
└── go.mod                      # Go module dependencies
```

## 🚀 Quick Start

### Prerequisites

- **Go** 1.24.4 or higher
- **PostgreSQL** 12 or higher

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/siti-nabila/grpc-auth.git
   cd grpc-auth
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Configure environment**
   
   Copy `env.example.yaml` to `env.yaml`, then fill in local database credentials,
   Ed25519 issuer key paths, a separate limiter HMAC secret, TLS certificate/key
   paths and service caller digest/expiry.
   See [service authentication](docs/service-authentication.md) for credential
   provisioning, allowlists and rotation. `env.yaml` is ignored by Git:
   ```bash
   cp env.example.yaml env.yaml
   ```
   ```yaml
   Database:
     User:
       User: postgres
       Password: "" # Set locally; never commit credentials
       Host: localhost
       Port: 5432
       Name: user
       Driver: postgres
   ```

4. **Build the application**
   ```bash
   make build
   ```

5. **Run the server**
   ```bash
   make run
   ```

   Server will start on `localhost:50051`

## 🔧 Available Commands

### Build & Run

```bash
# Build application
make build

# Run application
make run

# Build and run
make all

# Clean build artifacts
make clean
```

### Protocol Buffers

The versioned `UserService` and `ProfileService` contracts are consumed from
`github.com/siti-nabila/api-contracts`. Protobuf definitions and generated Go
packages are maintained in the separate `api-contracts` repository.

## 📡 API Usage

The default server composes all six RPCs with the session foundation pipeline.
Legacy HS256 and sid-less tokens are rejected. Register returns PENDING metadata
and an empty token; activation remains an administrator workflow. Initial refresh
issuance does not provide a RefreshSession endpoint. Backend wiring is complete;
[the external deployment gate](docs/foundation-activation-readiness.md) remains open.

### Register User

```bash
grpcurl -cacert "$AUTH_CA_FILE" -H "x-service-token: $SERVICE_TOKEN" \
  -d '{"email": "user@example.com", "password": "password123"}' \
  localhost:50051 user.v1.UserService/Register
```

### Login

```bash
grpcurl -cacert "$AUTH_CA_FILE" -H "x-service-token: $SERVICE_TOKEN" \
  -d '{"email": "user@example.com", "password": "password123"}' \
  localhost:50051 user.v1.UserService/Login
```

### Test RPC

```bash
grpcurl -cacert "$AUTH_CA_FILE" -H "x-service-token: $SERVICE_TOKEN" \
  localhost:50051 user.v1.UserService/TesRPC
```

The RPC examples require a target hostname matching the server certificate SAN.
`AUTH_CA_FILE` and `SERVICE_TOKEN` refer to values supplied locally by the caller
owner; do not commit them.

## 🗄️ Database Setup

Use the auth/session/RBAC/audit/outbox schema specified in
[the management spec](docs/user-management-spec.md). Startup requires the primary
`Database.User` PostgreSQL pool and does not run migrations or seed an admin.
Verify schema and a minimal ACTIVE administrator before deployment through the
[readiness checklist](docs/foundation-activation-readiness.md).

## 📝 Configuration

### env.yaml

```yaml
ApplicationName: USER_AUTH_SERVICE
Environment: development
DebugMode: false
Port: 50051                    # gRPC server port
Host: localhost
Timeout: 5s
KeepAlive: 2m
KeepAliveTimeout: 3s
KeepAliveIdle: 15m

Database:
  User:
    User: postgres
    Password: "" # Set locally; never commit credentials
    Host: localhost
    Port: 5432
    Name: user
    Driver: postgres

AccessIssuer:
  PrivateKeyFile: "" # PKCS#8 Ed25519 private PEM path
  PublicKeyFile: "" # Matching PKIX public PEM path
  KeyVersion: ""
AuthLimiter:
  AccountHMACSecret: "" # Separate private secret

Logger:
  HTTPMode: json               # or 'text'
  DBMode: text                 # or 'json'
```

## 📦 Key Packages

### `pkg/database`
- **DBLogger** - Database query wrapper with logging
- **postgres.go** - PostgreSQL connection management
- **helper.go** - Query interpolation and formatting

### `api-contracts/pkg/dictionary`
- Shared error registry and multi-language definitions
- Namespaced auth/common errors consumed directly by this service

### `pkg/jwt`
- JWT token generation and validation
- Claims management

### `pkg/logger`
- Structured logging with logrus
- File rotation and compression
- Multi-level logging support

## 🔐 Security

- TLS and separate opaque service authentication for every RPC
- EdDSA access tokens backed by authoritative primary session state
- PENDING registration without credentials; Login only for ACTIVE accounts
- Argon2id password storage and atomic session/refresh/audit creation
- Primary permission checks and locked self-profile updates
- Reused database pools; sensitive foundation SQL debugging disabled

## 📊 Logging

Logs are automatically created in the `logs/` directory:
- `logs/http-*.log` - HTTP/gRPC request logs
- `logs/db-*.log` - Database query logs

Logs rotate daily and are compressed after 30 days.

## 🐛 Error Handling

The error registry in `github.com/siti-nabila/api-contracts/pkg/dictionary` is
the source of truth for error keys, codes, protocol status, and localized
messages. This service consumes the `dictionary/auth` and `dictionary/common`
namespaces directly. Database-specific errors are normalized locally and then
mapped to the corresponding shared registry error.

Clients select an arbitrary catalog language through gRPC metadata
`x-language`. Locale tags use BCP-47, and regional values such as `zh-CN`
fall back to their base catalog entry (`zh`). Validation errors added through
`dictionary.FieldErrors.Add` preserve every message for the same field.

## 📚 Proto Definitions

All versioned user, profile, and paginator contracts live in the shared
[`api-contracts`](https://github.com/siti-nabila/api-contracts) module.

## 🚦 Development Workflow

1. Modify shared contracts and generate code in `api-contracts`
2. Use the parent `go.work` to test local contract changes without publishing
3. Implement handlers in `internal/handler/`
4. Implement business logic in `internal/features/`
5. Build and test with `make build` and `go test ./...`
6. Publish `api-contracts`, update `go.mod`, then test with `GOWORK=off`

## 📄 License

This project is for practice purposes.

## 👤 Author

**Siti Nabila**

---

**Last Updated:** 2025
