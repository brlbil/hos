// SPDX-License-Identifier: MIT

// Package hos provides a distributed object storage system with replication,
// encryption, and access control features. The system consists of three main
// entities: Users (who own resources), Pools (containers for objects), and
// Objects (the actual data with metadata).
//
// The storage system supports:
//   - Multi-server replication with configurable replica counts
//   - Server-side encryption
//   - Fine-grained permission control at pool levels
//   - Pool linking for creating aliases and sharing objects between users
//   - Metadata and labeling system for objects and pools
//
// Core relationships:
//   - Users own Pools and Objects
//   - Pools contain Objects and define replication/encryption policies
//   - Objects inherit settings from their parent Pool
//   - Pools can be linked to other Pools for sharing and aliasing
package hos

import (
	"io"
	"net/url"
	"time"

	"github.com/brlbil/hos/pkg/crypto"
)

// Key represents an encryption key associated with a user in the home object storage.
// Keys are used for server-side encryption of objects.
// Each key has a cryptographic signature for verification and is tied to a specific user.
type Key struct {
	// Signature is the cryptographic signature of the key, used for verification
	// and used as unique ID for storing the keys
	Signature string `json:"signature" yaml:"signature" print:"default,wide"`

	// UserID identifies which user this key belongs to
	UserID string `json:"user_id" yaml:"userID" print:"wide"`

	// CreatedAt tracks when this key was generated. This field is ignored in diff operations
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"createdAt,omitempty" diff:"ignore"`
}

// User represents a user account in the home object storage.
// Users are the primary security boundary - they own pools and objects,
// have their own encryption keys, and can be granted permissions to access
// resources owned by other users. The system supports admin users who can
// impersonate other users for administrative operations.
type User struct {
	// ID is the unique identifier for this user, generated
	// deterministically from the user's name
	ID string `json:"id" yaml:"id" print:"default,wide"`

	// Name is the human-readable username. Must be unique within the system
	// and is used for authentication and permission grants
	Name string `json:"name" yaml:"name" print:"default,wide"`

	// PublicKeys contains the cryptographic public keys associated with this user.
	// Used for authentication, and authorization operations.
	// Users can have multiple keys for key rotation scenarios or for connecting from multiple devices
	PublicKeys []crypto.PublicKey `json:"public_keys" yaml:"publicKey,omitempty" print:"wide"`

	// admin indicates if this user is currently acting on behalf of another user
	admin bool
}

// IsOnBehalf returns true if this user is currently acting on behalf of another user.
// This is used for admin impersonation where an admin user can perform
// operations as if they were a different user
func (u *User) IsOnBehalf() bool {
	return u.admin
}

// OnBehalf marks this user as acting on behalf of another user.
// This is typically called when an admin user is impersonating another user
// for administrative operations
func (u *User) OnBehalf() {
	u.admin = true
}

// Pool represents a storage container that holds objects in the home object storage.
// Pools define policies for replication, encryption, and access control that are
// inherited by all objects stored within them. Pools can be linked to other pools
// to create aliases and sharing across users.
//
// Struct tags explanation:
//   - boltholdKey: Specifies the primary key field for database storage
//   - parentID: Indicates this field creates a parent-child relationship
//   - header: Maps the field to HTTP headers for API communication
//   - print: Controls which fields are displayed in CLI output (default/wide modes)
//   - diff: Fields marked "ignore" are excluded from comparison operations
type Pool struct {
	// ID is the unique identifier for this pool, generated deterministically
	// from the owner's UserID and pool name. Used as primary key in storage
	ID string `json:"id" yaml:"id" boltholdKey:"ID" header:"X-Hos-Pool-Id" print:"default,wide"`

	// UserID identifies the owner of this pool. Creates a parent-child relationship
	// where the user is the parent and pools are children
	UserID string `json:"user_id,omitempty" yaml:"userID,omitempty" boltholdIndex:"UserID" parentID:"true" header:"X-Hos-User-Id" print:"wide"`

	// Name is the human-readable name of the pool. Combined with UserID to generate
	// the unique ID. Must be unique within the user's namespace
	Name string `json:"name" yaml:"name" header:"X-Hos-Pool-Name" print:"default,wide"`

	// ReplicaCount specifies how many copies of each object should be maintained
	// across the system. Objects inherit this setting
	ReplicaCount int `json:"replica_count,omitempty" yaml:"replicaCount" header:"X-Hos-Replica-Count" print:"default,wide"`

	// ObjectCount tracks the number of objects currently stored in this pool.
	// Maintained automatically and excluded from diff operations as it's computed
	ObjectCount int `json:"object_count,omitempty" yaml:"objectCount" header:"X-Hos-Object-Count,omitempty" diff:"ignore" print:"wide"`

	// CreatedAt records when this pool was first created. Excluded from diff
	// operations as it may be different for every Pool on different servers
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"createdAt,omitempty" header:"X-Hos-Created,omitempty" diff:"ignore"`

	// ModifiedAt tracks the last time this pool's metadata was updated.
	// Automatically maintained and excluded from diff operations
	ModifiedAt time.Time `json:"modified_at,omitempty" yaml:"modifiedAt,omitempty" header:"Last-Modified,omitempty" diff:"ignore" print:"wide"`

	// Size tracks the total bytes used by all objects in this pool.
	// Maintained automatically and excluded from diff operations as it's computed
	Size int64 `json:"size,omitempty" yaml:"size" header:"X-Hos-Pool-Bytes-Used,omitempty" diff:"ignore" print:"default,wide"`

	// Encrypted indicates whether objects in this pool should be encrypted.
	// When true, objects inherit encryption settings
	Encrypted bool `json:"encrypted,omitempty" yaml:"encrypted" header:"X-Hos-Encrypted,omitempty" print:"wide"`

	// Hash is a cryptographic hash of the pool's metadata, used for integrity
	// checking and change detection. Excluded from diff as it's computed
	Hash string `json:"hash,omitempty" yaml:"hash,omitempty" header:"ETag,omitempty" diff:"ignore"`

	// Attributes stores arbitrary key-value metadata for this pool.
	// Attributes can only be added, they cannot be deleted nor altered after.
	// Can be used for custom application-specific information
	Attributes map[string]string `json:"attributes,omitempty" yaml:"attributes,omitempty" header:"X-Hos-Attributes,omitempty"`

	// Labels stores user-defined key-value pairs for organizing and filtering pools.
	// Used for queries, searches, and bulk operations across multiple pools
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty" header:"X-Hos-Labels,omitempty"`

	// Permissions defines who can access this pool and what operations they can perform.
	// Maps user names (or "*" for everyone) to Permission values ("r", "w")
	Permissions map[string]Permission `json:"permissions,omitempty" yaml:"permissions,omitempty" header:"X-Hos-Permissions,omitempty"`

	// LinkedID points to another pool that this pool is linked to, creating an
	// alias or inheritance relationship
	LinkedID string `json:"linked_id,omitempty" yaml:"linkedID,omitempty" header:"X-Hos-Linked-Id,omitempty" print:"wide"`
}

// Object represents a data object stored in the home object storage.
// Objects are the fundamental data units that contain the actual file content
// along with associated metadata. Objects belong to a Pool and inherit policies
// like replication and encryption from their parent pool.
// Objects metadata is mutable but data is immutable, to make changes the data
// object needs to be replaced
type Object struct {
	// ID is the unique identifier for this object, generated deterministically
	// from the PoolID and object name. Used as primary key in storage
	ID string `json:"id" yaml:"id" boltholdKey:"ID" header:"X-Hos-Object-Id" print:"default,wide"`

	// PoolID identifies which pool this object belongs to. Creates a parent-child
	// relationship where the pool is the parent and objects are children
	PoolID string `json:"pool_id,omitempty" yaml:"poolID,omitempty" boltholdIndex:"PoolID" parentID:"true" header:"X-Hos-Pool-Id" print:"wide"`

	// UserID identifies the owner of this object. Usually matches the pool's
	// UserID but can differ in shared pool scenarios
	UserID string `json:"user_id,omitempty" yaml:"userID,omitempty" boltholdIndex:"UserID" header:"X-Hos-User-Id" print:"wide"`

	// Name is the human-readable name of the object within its pool.
	// Combined with PoolID to generate the unique ID
	Name string `json:"name" yaml:"name" header:"X-Hos-Object-Name" print:"default,wide"`

	// ContentType specifies the MIME type of the object's content (e.g., "image/jpeg").
	ContentType string `json:"content_type" yaml:"contentType" header:"Content-Type" print:"wide"`

	// CreatedAt records when this object was first stored. Immutable timestamp
	// excluded from diff operations
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"createdAt,omitempty" header:"X-Hos-Created,omitempty" diff:"ignore"`

	// ModifiedAt tracks the last time this object's metadata was updated.
	// Automatically maintained and excluded from diff operations
	ModifiedAt time.Time `json:"modified_at,omitempty" yaml:"modifiedAt,omitempty" header:"Last-Modified,omitempty" diff:"ignore" print:"wide"`

	// ReplicaCount specifies how many copies of this object should be maintained.
	// It inherits the value from the parent pool
	ReplicaCount int `json:"replica_count,omitempty" yaml:"replicaCount,omitempty" header:"X-Hos-Replica-Count" print:"default,wide"`

	// Size is the byte size of the object's content. Required for storage
	// allocation and transfer verification
	Size int64 `json:"size" yaml:"size" header:"X-Hos-Content-Length" print:"default,wide"`

	// Encrypted indicates whether this object's content is encrypted.
	// It inherits the encryption setting from the parent pool
	Encrypted bool `json:"encrypted,omitempty" yaml:"encrypted" header:"X-Hos-Encrypted,omitempty" print:"wide"`

	// Hash is a cryptographic hash of the object's content, used for integrity
	// verification and change detection (ETag support)
	Hash string `json:"hash,omitempty" yaml:"hash,omitempty" header:"ETag,omitempty"`

	// Labels stores user-defined key-value pairs for organizing and filtering objects.
	// Used for queries, searches, and bulk operations
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty" header:"X-Hos-Labels,omitempty"`

	// body holds the actual content stream of the object. Used internally
	// to implement io.ReadCloser interface for streaming data access
	body io.ReadCloser

	// server tracks which server provided this object in the response.
	// TODO: could there be another way to pass this information so we can remove this
	server string `diff:"ignore"`
}

// Compile-time check to ensure Object implements io.ReadCloser interface.
var _ io.ReadCloser = &Object{}

// SetBody attaches a content stream to this object.
func (o *Object) SetBody(rc io.ReadCloser) {
	o.body = rc
}

// Read implements the io.Reader interface by delegating to the underlying content stream.
func (o *Object) Read(p []byte) (int, error) {
	return o.body.Read(p)
}

// Close implements the io.Closer interface by closing the underlying content stream.
func (o *Object) Close() error {
	return o.body.Close()
}

// SetServerAddr records which server provided this object.
func (o *Object) SetServerAddr(s string) {
	o.server = s
}

// ServerAddr returns the address of the server that provided this object.
func (o *Object) ServerAddr() string {
	return o.server
}

// Usage represents resource usage statistics for a user account.
// This provides a summary of how much storage and how many resources
// a user is currently consuming across the distributed system.
type Usage struct {
	// Name is the username this usage report belongs to
	Name string `json:"name,omitempty" print:"default,wide"`

	// Pools is the number of pools owned by this user
	Pools int `json:"pools,omitempty" print:"wide"`

	// Object is the total number of objects owned by this user across all pools
	Object int `json:"objects,omitempty" print:"wide"`

	// Size is the total bytes consumed by this user's objects across all pools
	Size int64 `json:"size,omitempty" print:"default,wide"`
}

// Statfs represents filesystem statistics from the underlying storage.
// This mirrors the Unix statfs system call, providing information about
// disk space and inode usage on the storage backend. Used for capacity
// planning and monitoring storage health.
type Statfs struct {
	// BlockSize is the size in bytes of each filesystem block
	BlockSize uint32 `header:"X-Hos-Disk-BlockSize,omitempty"`

	// Blocks is the total number of blocks in the filesystem
	Blocks uint64 `header:"X-Hos-Disk-Blocks,omitempty"`

	// BlocksFree is the number of free blocks available to privileged users
	BlocksFree uint64 `header:"X-Hos-Disk-BlocksFree,omitempty"`

	// BlocksAvailable is the number of free blocks available to unprivileged users
	BlocksAvailable uint64 `header:"X-Hos-Disk-BlocksAvailable,omitempty"`

	// Inodes is the total number of inodes (file entries) in the filesystem
	Inodes uint64 `header:"X-Hos-Disk-Inodes,omitempty"`

	// InodesFree is the number of free inodes available for new files
	InodesFree uint64 `header:"X-Hos-Disk-InodesFree,omitempty"`
}

// ServerInfo represents comprehensive information about a server.
// This includes the server's network location, current operation count, and underlying
// storage statistics. Used for health monitoring, load balancing, and capacity planning.
type ServerInfo struct {
	// URL is the network address where this server can be reached
	URL *url.URL

	// Operations counts the number of operations this server has processed.
	// Used for load balancing and performance monitoring
	Operations int64 `header:"X-Hos-Operations-Count,omitempty"`

	// Statfs embeds filesystem statistics for this server's storage backend
	Statfs
}

// FreeDisk calculates and returns the amount of free disk space in bytes
// available to unprivileged users on this server. This is computed by
// multiplying the block size by the number of available blocks.
func (s *ServerInfo) FreeDisk() uint64 {
	return uint64(s.BlockSize) * s.BlocksAvailable
}

// Permission represents an access permission level in the storage system.
// Permissions control what operations users can perform on pools and objects.
// Valid values are:
//   - "r": Read-only access (can get objects and list contents)
//   - "w": Read-write access (can perform all operations)
//
// Permissions are assigned in Pool.Permissions map, where keys are usernames
// (or "*" for everyone) and values are Permission levels.
type Permission string
