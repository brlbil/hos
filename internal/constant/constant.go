// SPDX-License-Identifier: MIT

// Package constant defines system-wide constants for HOS.
// It includes HTTP status codes, user identifiers, API paths, and environment variables.
package constant

// HTTPStatusNotEqual returned when same object has different fields
const HTTPStatusNotEqual = 433

// HTTPStatusNotInitialized returned when an cluster is not initialized
const HTTPStatusNotInitialized = 434

// HTTPStatusNotAllowed returned when an operation is not allowed
const HTTPStatusNotAllowed = 444

// AdminUser admin account name
const AdminUser = "admin"

// AnonUser admin account name
const AnonUser = ""

// Everyone represents permission for any user
const Everyone = "*"

// APIPrefix api path for user management
const APIPrefix = "/api/v1"

// UserAPIPrefix api path for user management
const UserAPIPrefix = "/api/v1/user"

// KeyAPIPrefix api path for key management
const KeyAPIPrefix = "/api/v1/key"

// KeyAPIDataPrefix api path for key data management
const KeyAPIDataPrefix = "/api/v1/key/data"

// Environment variables

// EnvPassword get password for encryption keys from environment
const EnvPassword = "HOS_PASSWORD"

// EnvNewPassword get new password for encryption keys from environment
// this is used for creating new key from and existing one need to be
// provided with EnvPassword and EnvCreateKey
const EnvNewPassword = "HOS_NEW_PASSWORD"

// EnvCreateKey confirms creation of a key from environment variables
const EnvCreateKey = "HOS_CREATE_KEY"
