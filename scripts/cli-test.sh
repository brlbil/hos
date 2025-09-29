#!/bin/bash

# HOS CLI Test Script
# Tests end-to-end functionality with real servers

set -e  # Exit on error

# Configuration
CLUSTER_NAME="local"
DEFAULT_USER="test"
SERVER1_ADDR="127.0.0.1:1981"
SERVER2_ADDR="127.0.0.1:1982"
TEST_DIR="./tmp/hos-cli-test"
SERVER1_DATA="$TEST_DIR/server1"
SERVER2_DATA="$TEST_DIR/server2"
CLIENT_DATA="$TEST_DIR/client"
TEST_FILES_DIR="$TEST_DIR/test-files"
CONFIG_FILE="$TEST_DIR/hos-config.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup on exit - only if successful
cleanup_on_exit() {
    local exit_code=$1

    # Always kill servers
    if [[ -f "$TEST_DIR/server1.pid" ]]; then
        log_info "Stoping server1"
        kill $(cat "$TEST_DIR/server1.pid") 2>/dev/null || true
        rm -f "$TEST_DIR/server1.pid"
    fi

    if [[ -f "$TEST_DIR/server2.pid" ]]; then
        log_info "Stoping server2"
        kill $(cat "$TEST_DIR/server2.pid") 2>/dev/null || true
        rm -f "$TEST_DIR/server2.pid"
    fi

    # Only remove test directory if tests passed
    if [[ $exit_code -eq 0 ]]; then
        log_info "Tests passed - cleaning up test directory..."
        rm -rf $(dirname "$TEST_DIR")
        log_info "Cleanup complete"
    else
        log_warning "Tests failed - keeping test directory for debugging: $TEST_DIR"
        log_info "Check server logs: $TEST_DIR/server1.log and $TEST_DIR/server2.log"
    fi
}

# Trap cleanup only on successful exit
trap 'cleanup_on_exit $?' EXIT

# Setup test environment
setup_test_env() {
    log_info "Setting up test environment..."

    # Clean up any existing test directory first
    if [[ -d "$TEST_DIR" ]]; then
        log_info "Cleaning up existing test directory..."
        rm -rf "$TEST_DIR"
    fi

    # Create test directories
    mkdir -p "$SERVER1_DATA" "$SERVER2_DATA" "$CLIENT_DATA" "$TEST_FILES_DIR"

    # Create test files
    echo "Hello World" > "$TEST_FILES_DIR/hello.txt"
    echo "Test document content" > "$TEST_FILES_DIR/document.md"
    mkdir -p "$TEST_FILES_DIR/photos"
    echo "Photo 1 content" > "$TEST_FILES_DIR/photos/photo1.jpg"
    echo "Photo 2 content" > "$TEST_FILES_DIR/photos/photo2.jpg"

    # Config file will be specified with --config-file flag

    log_success "Test environment setup complete"
}

# Start server
start_server() {
    local data_dir="$1"
    local address="$2"
    local pid_file="$3"

    log_info "Starting server at $address with data dir $data_dir..."

    # Start server in background with debug logging
    local log_file="${pid_file%.pid}.log"
    ./bin/hosd "$data_dir" --address "$address" --log debug > "$log_file" 2>&1 &
    local server_pid=$!
    echo "$server_pid" > "$pid_file"


    # Wait for server to be ready using health check
    local max_wait=30
    local wait_count=0
    local health_url="https://$address/healthz"

    log_info "Waiting for server to be ready at $health_url..."

    while [[ $wait_count -lt $max_wait ]]; do
        # Check if process is still running
        if ! kill -0 "$server_pid" 2>/dev/null; then
            log_error "Server process died at $address"
            return 1
        fi

        # Check health endpoint (ignore SSL certificate issues)
        if curl -k -s -f "$health_url" >/dev/null 2>&1; then
            log_success "Server started and ready at $address (PID: $server_pid)"
            return 0
        fi

        sleep 1
        ((wait_count++))
    done

    log_error "Server at $address did not become ready within ${max_wait} seconds"
    return 1
}

# Test command with expected success
test_success() {
    local cmd="$1"
    local description="$2"

    log_info "Testing: $description"
    log_info "Command: $cmd"

    # Execute command and capture both stdout and stderr
    if eval "$cmd"; then
        log_success "$description - PASSED"
        return 0
    else
        log_error "$description - FAILED"
        return 1
    fi
}

# Test command with expected failure
test_failure() {
    local cmd="$1"
    local description="$2"

    log_info "Testing: $description (expecting failure)"
    log_info "Command: $cmd"

    if eval "$cmd" 2>/dev/null; then
        log_error "$description - FAILED (expected failure but succeeded)"
        return 1
    else
        log_success "$description - PASSED (failed as expected)"
        return 0
    fi
}

# Test command with JSON output and field expectation
expect_value() {
    local cmd="$1"
    local field_name="$2"
    local expected_value="$3"
    local description="$4"

    log_info "Testing: $description"
    log_info "Command: $cmd -o json"
    log_info "Expected field '$field_name' = '$expected_value'"

    # Check if jq is available
    if ! command -v jq >/dev/null 2>&1; then
        log_warning "jq tool not found, skipping field validation test"
        return 0
    fi

    # Execute command with JSON output and extract field
    local output
    output=$(eval "$cmd -o json" 2>/dev/null)
    local exit_code=$?

    if [[ $exit_code -ne 0 ]]; then
        log_error "$description - FAILED (command failed with exit code: $exit_code)"
        return 1
    fi

    # Extract field value using jq
    local actual_value
    actual_value=$(echo "$output" | jq -r ".$field_name // empty")

    if [[ "$actual_value" == "$expected_value" ]]; then
        log_success "$description - PASSED (field '$field_name' = '$actual_value')"
        return 0
    else
        log_error "$description - FAILED (field '$field_name' = '$actual_value', expected '$expected_value')"
        return 1
    fi
}



# Main test sequence
run_tests() {
    log_info "Starting HOS CLI tests..."

    # Setup
    log_info "=== Setup Environment ==="
    setup_test_env

    # Start first server
    log_info "=== Start First Server ==="
    start_server "$SERVER1_DATA" "$SERVER1_ADDR" "$TEST_DIR/server1.pid"
    start_server "$SERVER2_DATA" "$SERVER2_ADDR" "$TEST_DIR/server2.pid" 

    # Initialize cluster with first server
    log_info "=== Initialize Clusters ==="
    test_success "./bin/hos --config-file $CONFIG_FILE init $SERVER1_ADDR --cluster-name $CLUSTER_NAME --default-user $DEFAULT_USER" "Initialize cluster with first server"


    # Basic pool operations
    log_info "=== Pool Operations ==="
    test_success "./bin/hos --config-file $CONFIG_FILE make-pool Documents" "Create Documents pool"
    test_success "./bin/hos --config-file $CONFIG_FILE make-pool --encrypted -a dest=cloud Photos" "Create encrypted Photos pool"
    expect_value "./bin/hos --config-file $CONFIG_FILE stat Photos" "attributes.dest" "cloud" "List pools"
    test_success "./bin/hos --config-file $CONFIG_FILE list" "List pools"

    # Changing attribute not allowed
    test_failure "./bin/hos --config-file $CONFIG_FILE attr Photos dest=water" "Set existing attribute on pool"

    # Object upload operations
    log_info "=== Object Upload Operations ==="
    test_success "./bin/hos --config-file $CONFIG_FILE upload --silent $TEST_FILES_DIR/hello.txt Documents" "Upload single file"
    test_success "./bin/hos --config-file $CONFIG_FILE upload --silent $TEST_FILES_DIR/document.md Documents --label type=doc" "Upload document with label"

    # Upload to encrypted pool using environment variables
    log_info "Testing encrypted pool upload with environment variables..."
    test_success "HOS_PASSWORD=testpass123 HOS_CREATE_KEY=y ./bin/hos --config-file $CONFIG_FILE upload --silent -R $TEST_FILES_DIR/photos/ Photos" "Upload directory to encrypted pool"

    # Object listing and inspection
    log_info "=== Object Listing and Inspection ==="
    test_success "./bin/hos --config-file $CONFIG_FILE list Documents" "List objects in Documents pool"
    test_success "./bin/hos --config-file $CONFIG_FILE list Photos" "List objects in Photos pool"
    expect_value "./bin/hos --config-file $CONFIG_FILE stat Documents/document.md" "labels.type" "doc" "Get object stats"

    # Object download operations
    log_info "=== Object Download Operations ==="
    mkdir -p "$TEST_DIR/downloads"
    test_success "./bin/hos --config-file $CONFIG_FILE download --silent Documents/hello.txt $TEST_DIR/downloads/" "Download single file"
    test_success "./bin/hos --config-file $CONFIG_FILE download --silent Documents/... $TEST_DIR/downloads/" "Download all files from pool"

    # Expected failure tests
    log_info "=== Expected Failure Tests ==="
    test_failure "./bin/hos --config-file $CONFIG_FILE make-pool Failure -a wrong+key=1" "Create pool with wrong attribute (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE make-pool Failure -l w+k=val" "Create pool with wrong label key (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE make-pool Failure -l key=v%l" "Create pool with wrong label value (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE attr Failure dest=cloud" "Set attribute on not exitst pool (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE upload --silent $TEST_FILES_DIR/hello.txt Documents" "Upload duplicate file (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE make-pool Documents" "Create duplicate pool (should fail)"
    test_failure "./bin/hos --config-file $CONFIG_FILE download --silent NonExistent/file.txt $TEST_DIR/downloads/" "Download non-existent file (should fail)"

    # Add seconf server cluster
    log_info "=== Add Second Server To Cluster ==="
    test_success "./bin/hos --config-file $CONFIG_FILE init $SERVER2_ADDR --cluster-name $CLUSTER_NAME --default-user $DEFAULT_USER" "Initialize the second server"

    # Make pool to sync new server with wrong options
    test_failure "./bin/hos --config-file $CONFIG_FILE make-pool Documents --label will=fail" "Create documents pool on missing server with wrong label (should fail)"
    test_success "./bin/hos --config-file $CONFIG_FILE make-pool Documents" "Create documents pool on missing server"

    # Test replication with second server
    log_info "=== Test Multi-Server Operations ==="
    test_success "./bin/hos --config-file $CONFIG_FILE make-pool --replica-count 2 Replicated" "Create replicated pool"
    test_success "./bin/hos --config-file $CONFIG_FILE upload --silent $TEST_FILES_DIR/document.md Replicated" "Upload to replicated pool"
    test_success "./bin/hos --config-file $CONFIG_FILE list Replicated" "List replicated pool objects"

    # Metadata operations
    log_info "=== Advanced Operations ==="
    test_success "./bin/hos --config-file $CONFIG_FILE label Documents/hello.txt type=text project=test" "Add labels to object"
    mkdir -p $TEST_DIR/downloads/labeled
    test_success "./bin/hos --config-file $CONFIG_FILE download --silent --label type==text Documents $TEST_DIR/downloads/labeled/" "Download with label selector"
    test_failure "./bin/hos --config-file $CONFIG_FILE move Documents/document.md Replicated/moved-document.md" "Move object between pools with wrong replication count (should fail)"
    test_success "./bin/hos --config-file $CONFIG_FILE make-pool NewDocs" "Create new_docs pool"
    test_success "./bin/hos --config-file $CONFIG_FILE move Documents/document.md NewDocs/moved-document.md" "Move object between pools"

    # User and key management
    log_info "=== User and Key Management ==="
    test_success "./bin/hos --config-file $CONFIG_FILE user list" "List users"
    test_success "./bin/hos --config-file $CONFIG_FILE key list" "List encryption keys"

    # Cleanup operations
    log_info "=== Cleanup Operations ==="
    test_success "./bin/hos --config-file $CONFIG_FILE remove Documents/hello.txt" "Remove single object"
    test_success "./bin/hos --config-file $CONFIG_FILE remove --recursive --force Photos" "Remove pool recursively, server2 does not have it force it"

    log_success "All CLI tests completed successfully!"
}


# Usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -h, --help          Show this help message"
    echo ""
    echo "This script tests HOS CLI functionality end-to-end by:"
    echo "1. Starting test servers"
    echo "2. Initializing a cluster"
    echo "3. Testing various HOS commands"
    echo "4. Testing both success and failure scenarios"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Main execution
main() {
    log_info "HOS CLI Test Script"
    log_info "=================="

    # Check if binaries exist
    if [[ ! -f "./bin/hos" ]] || [[ ! -f "./bin/hosd" ]]; then
        log_error "HOS binaries not found. Please run 'make build' first."
        exit 1
    fi

    run_tests
}

# Run main function
main
