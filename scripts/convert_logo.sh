#!/bin/bash
# Script to convert logo.svg to logo.png for watermarking

echo "Converting logo.svg to logo.png..."

# Check if ImageMagick is installed
if command -v magick &> /dev/null; then
    # ImageMagick 7+
    magick convert -background none -density 300 assets/logo.svg -resize 800x assets/logo.png
    echo "✓ Conversion successful using ImageMagick 7"
elif command -v convert &> /dev/null; then
    # ImageMagick 6
    convert -background none -density 300 assets/logo.svg -resize 800x assets/logo.png
    echo "✓ Conversion successful using ImageMagick 6"
elif command -v inkscape &> /dev/null; then
    # Inkscape
    inkscape assets/logo.svg --export-type=png --export-filename=assets/logo.png --export-width=800
    echo "✓ Conversion successful using Inkscape"
else
    echo "❌ Error: No SVG converter found!"
    echo ""
    echo "Please install one of the following:"
    echo "  - ImageMagick: https://imagemagick.org/script/download.php"
    echo "  - Inkscape: https://inkscape.org/release/"
    echo ""
    echo "Or manually convert assets/logo.svg to assets/logo.png (recommended size: 800px width)"
    exit 1
fi

# Check if conversion was successful
if [ -f "assets/logo.png" ]; then
    size=$(wc -c < "assets/logo.png")
    echo "✓ PNG file created: assets/logo.png (${size} bytes)"
else
    echo "❌ Conversion failed"
    exit 1
fi
