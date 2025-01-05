#!/bin/zsh

# Define the output file name
output_file="all_go_files.txt"

# Check if the output file already exists and remove it
if [[ -f "$output_file" ]]; then
  echo "Removing existing $output_file"
  rm "$output_file"
fi

# Loop through all .go files in the current directory, excluding files with _test in their name
for file in *.go; do
  if [[ -f "$file" && "$file" != *_test.go ]]; then
    echo "Processing $file"
    echo "\n// File: $file\n" >> "$output_file"
    cat "$file" >> "$output_file"
    echo "\n" >> "$output_file"
  fi
done

# Notify completion
if [[ -f "$output_file" ]]; then
  echo "All .go files have been concatenated into $output_file"
else
  echo "No .go files found in the directory."
fi
