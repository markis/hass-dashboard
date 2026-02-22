#!/bin/sh
set -e

FONTS_DIR="static/fonts"
OUTPUT_FILE="static/fonts/weather-icons-embedded.css"

echo "Converting fonts to base64 data URIs..."

# Read the base CSS
cp "${FONTS_DIR}/weather-icons.min.css" "${OUTPUT_FILE}"

# Convert WOFF2 to base64
# Linux uses -w 0, macOS uses -b 0 (or just base64 with input redirection)
WOFF2_BASE64=$(base64 -w 0 "${FONTS_DIR}/weathericons-regular-webfont.woff2" 2>/dev/null || base64 -b 0 -i "${FONTS_DIR}/weathericons-regular-webfont.woff2" 2>/dev/null || base64 -i "${FONTS_DIR}/weathericons-regular-webfont.woff2" | tr -d '\n')

# Convert WOFF to base64
WOFF_BASE64=$(base64 -w 0 "${FONTS_DIR}/weathericons-regular-webfont.woff" 2>/dev/null || base64 -b 0 -i "${FONTS_DIR}/weathericons-regular-webfont.woff" 2>/dev/null || base64 -i "${FONTS_DIR}/weathericons-regular-webfont.woff" | tr -d '\n')

# Create embedded CSS with data URIs
cat > "${OUTPUT_FILE}" << EOF
@font-face {
  font-family: 'weathericons';
  src: url(data:font/woff2;charset=utf-8;base64,${WOFF2_BASE64}) format('woff2'),
       url(data:font/woff;charset=utf-8;base64,${WOFF_BASE64}) format('woff');
  font-weight: normal;
  font-style: normal;
}

.wi {
  display: inline-block;
  font-family: 'weathericons';
  font-style: normal;
  font-weight: normal;
  line-height: 1;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
EOF

# Append icon classes from original CSS (extract everything after the .wi{ definition)
# The CSS is minified, so we need to extract the icon classes differently
# We'll extract everything after the first occurrence of ".wi-"
sed -n 's/.*\(\.wi-fw{.*\)/\1/p' "${FONTS_DIR}/weather-icons.min.css" >> "${OUTPUT_FILE}" || true

echo "Embedded CSS created at ${OUTPUT_FILE}"
ls -lh "${OUTPUT_FILE}"
