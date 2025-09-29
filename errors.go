// SPDX-License-Identifier: MIT

package hos

import (
	"fmt"
)

// ConstError represents a constant error that implements the error interface.
// This type is used to define sentinel errors as compile-time constants,
// which allows for efficient error comparisons using == or errors.Is().
// ConstError values are immutable and comparable, making them ideal for
// defining a set of well-known errors that can be checked programmatically.
type ConstError string

// Error implements the error interface by returning the string representation of the constant error.
func (e ConstError) Error() string { return string(e) }

const (
	// ErrNotEmpty is returned when attempting to delete a Pool that still contains Objects.
	// Pools must be emptied of all objects before they can be deleted
	ErrNotEmpty = ConstError("not empty")

	// ErrNotExist is returned when a requested resource (User, Pool, or Object) does not exist.
	ErrNotExist = ConstError("not exist")

	// ErrExist is returned when attempting to create a resource that already exists.
	ErrExist = ConstError("already exist")

	// ErrNotEqual is returned when comparing Pools or Objects from different servers
	// that should be identical but have different content or metadata.
	ErrNotEqual = ConstError("not equal")

	// ErrNotAuthorized is returned when user authentication fails or when a user
	// attempts to access a resource they don't have permission to view
	ErrNotAuthorized = ConstError("not authorized")

	// ErrInsufficientPermissions is returned when a user is authenticated but lacks
	// the specific permissions needed to perform the requested operation
	ErrInsufficientPermissions = ConstError("insufficient permissions")

	// ErrNotInitialized is returned when a system component is accessed before proper
	// initialization, such as when a cluster admin user does not have a public key
	// configured or when required configuration is missing
	ErrNotInitialized = ConstError("not initialized")

	// ErrAdminOnly is returned when a non-admin user attempts to call an API endpoint
	// that is restricted to admin users only, such as server management operations
	ErrAdminOnly = ConstError("admin only")

	// ErrNotAllowed is returned when an operation is explicitly prohibited by policy,
	// such as attempting to move an object between pools that do not have same replication count
	// or attempting to delete the last encryption key
	ErrNotAllowed = ConstError("not allowed")

	// ErrBadRequest is returned when a request contains malformed or invalid data,
	// such as missing required fields, invalid IDs, or incorrectly formatted values
	ErrBadRequest = ConstError("bad request")

	// ErrSizeRequired is returned when creating or uploading an Object without
	// specifying the content size
	ErrSizeRequired = ConstError("size required")

	// ErrContentTypeRequired is returned when creating an Object without specifying
	// a MIME content type
	ErrContentTypeRequired = ConstError("content-type required")

	// ErrContentTooLarge is returned when the uploaded content exceeds size limits
	// imposed by the server configuration or storage constraints
	ErrContentTooLarge = ConstError("content too large")

	// ErrInsufficientResources is returned when a request cannot be completed due to
	// resource limitations, such as creating an object with replication count
	// exceeds the number of servers or exceeds the pool replication count
	ErrInsufficientResources = ConstError("insufficient resources")

	// ErrConnectionFailure is returned when network communication with one or more
	// servers in the distributed system fails
	ErrConnectionFailure = ConstError("connection failure")

	// ErrNotAllCopiesAvailable is returned when an object exists but not all of its
	// replicas are accessible
	ErrNotAllCopiesAvailable = ConstError("not all copies available")

	// ErrCorrupted is returned when an object or its replicas fail integrity checks,
	// have unexpected metadata, or when replication count is inconsistent with expectations
	ErrCorrupted = ConstError("corrupted")

	// ErrDecryption is returned when an encrypted object cannot be decrypted, possibly
	// due to incorrect keys, corrupted ciphertext
	ErrDecryption = ConstError("decryption failed")
)

// HTTPError represents an HTTP-specific error with an associated status code.
// This is used when the API returns error responses, allowing
// the client to understand both the nature of the error (via the message)
// and the HTTP semantics (via the status code).
type HTTPError struct {
	// Message is the human-readable error description explaining what went wrong
	Message string

	// Code is the HTTP status code associated with this error (e.g., 404, 500, etc.)
	Code int
}

// Error implements the error interface by formatting the HTTP error as a string
// that includes both the status code and error message.
func (ht *HTTPError) Error() string {
	return fmt.Sprintf("HTTP Error: (%d) %s", ht.Code, ht.Message)
}
