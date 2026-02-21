#!/bin/sh
set -e

# Health check script for hass-dashboard
# Checks if the output file exists and has been updated within the expected interval

# Default config path
CONFIG_PATH="/app/config.yaml"

# Parse arguments
while [ $# -gt 0 ]; do
  case "$1" in
    --config)
      CONFIG_PATH="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 [--config /path/to/config.yaml]" >&2
      exit 1
      ;;
  esac
done

# Check if config file exists
if [ ! -f "$CONFIG_PATH" ]; then
  echo "ERROR: Config file not found: $CONFIG_PATH" >&2
  exit 1
fi

# Check if yq is available
if ! command -v yq >/dev/null 2>&1; then
  echo "ERROR: yq is required but not installed" >&2
  exit 1
fi

# Extract output path from config
OUTPUT_PATH=$(yq eval '.output.path' "$CONFIG_PATH")
if [ -z "$OUTPUT_PATH" ] || [ "$OUTPUT_PATH" = "null" ]; then
  echo "ERROR: Could not read output.path from config" >&2
  exit 1
fi

# Extract refresh interval from config (in seconds)
REFRESH_INTERVAL=$(yq eval '.refresh_interval' "$CONFIG_PATH")
if [ -z "$REFRESH_INTERVAL" ] || [ "$REFRESH_INTERVAL" = "null" ]; then
  echo "ERROR: Could not read refresh_interval from config" >&2
  exit 1
fi

# Calculate staleness threshold (2x refresh interval)
STALENESS_THRESHOLD=$((REFRESH_INTERVAL * 2))

# Check if output file exists
if [ ! -f "$OUTPUT_PATH" ]; then
  echo "ERROR: Output file does not exist: $OUTPUT_PATH" >&2
  exit 1
fi

# Get current time and file modification time (in seconds since epoch)
CURRENT_TIME=$(date +%s)
FILE_MOD_TIME=$(stat -c %Y "$OUTPUT_PATH" 2>/dev/null || stat -f %m "$OUTPUT_PATH" 2>/dev/null)

if [ -z "$FILE_MOD_TIME" ]; then
  echo "ERROR: Could not get modification time for: $OUTPUT_PATH" >&2
  exit 1
fi

# Calculate file age
FILE_AGE=$((CURRENT_TIME - FILE_MOD_TIME))

# Check if file is stale
if [ "$FILE_AGE" -gt "$STALENESS_THRESHOLD" ]; then
  echo "ERROR: Output file is stale (age: ${FILE_AGE}s, threshold: ${STALENESS_THRESHOLD}s): $OUTPUT_PATH" >&2
  exit 1
fi

# Success - exit silently with 0
exit 0
