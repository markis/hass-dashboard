#!/bin/sh
set -e

WEATHER_ICONS_VERSION="2.0.10"
DOWNLOAD_DIR="/tmp/weather-icons"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "${SCRIPT_DIR}")"
OUTPUT_DIR="${PROJECT_DIR}/static/fonts"

echo "Downloading Weather Icons ${WEATHER_ICONS_VERSION}..."

# Download from CDN
mkdir -p "${DOWNLOAD_DIR}"
cd "${DOWNLOAD_DIR}"

# Download font files
curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/font/weathericons-regular-webfont.woff2" \
  -o weathericons-regular-webfont.woff2

curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/font/weathericons-regular-webfont.woff" \
  -o weathericons-regular-webfont.woff

# Download CSS
curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/css/weather-icons.min.css" \
  -o weather-icons.min.css

echo "Download complete. Files:"
ls -lh

# Copy to output directory
mkdir -p "${OUTPUT_DIR}"
cp *.woff* "${OUTPUT_DIR}/"
cp weather-icons.min.css "${OUTPUT_DIR}/"

echo "Weather Icons installed to ${OUTPUT_DIR}"
