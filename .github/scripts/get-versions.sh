#!/bin/sh
# get-versions.sh - Detects versions for dual release channel system
#
# This script:
# 1. Gets the latest production version from releases branch tags
# 2. Generates a beta version for the upcoming patch release with a
#    sequential, monotonically increasing beta counter
# 3. Outputs both versions as JSON
#
# Version Format:
# - Upcoming: v{MAJOR}.{MINOR}.{PATCH+1}
# - Beta:     v{UPCOMING_VERSION}-beta.{N}
# - Example:  stable v0.5.2 -> upcoming v0.5.3 -> beta v0.5.3-beta.1
#             next beta (before stable bump) -> v0.5.3-beta.2

set -eu

# Configuration
RELEASES_BRANCH="origin/releases"
TAG_PATTERN="v[0-9]*"

# Default fallback version (used when no tags found)
DEFAULT_VERSION="v0.0.0"

# Get the latest production version from releases branch.
# Production tags are semver only (no prerelease suffix).
get_prod_version() {
    # Fetch tags from the releases branch
    git fetch "${RELEASES_BRANCH}" 2>/dev/null || true

    # Latest stable tag: matches vX.Y.Z with no prerelease suffix
    latest_tag=$(git tag -l "${TAG_PATTERN}" --sort=-version:refname \
        | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
        | head -1)

    if [ -z "${latest_tag}" ]; then
        echo "${DEFAULT_VERSION}"
    else
        echo "${latest_tag}"
    fi
}

# Increment patch version for the upcoming release (e.g., v0.5.2 -> v0.5.3)
increment_patch_version() {
    prod_version="$1"
    major=$(echo "${prod_version}" | sed 's/^v\([0-9]*\)\..*/\1/')
    minor=$(echo "${prod_version}" | sed 's/^v[0-9]*\.\([0-9]*\)\..*/\1/')
    patch=$(echo "${prod_version}" | sed 's/^v[0-9]*\.[0-9]*\.\([0-9]*\).*/\1/')

    if [ -z "${major}" ] || [ -z "${minor}" ] || [ -z "${patch}" ]; then
        echo "${DEFAULT_VERSION}"
        return
    fi

    new_patch=$((patch + 1))
    echo "v${major}.${minor}.${new_patch}"
}

# Find the next beta counter for a given upcoming version.
# Looks at existing v{UPCOMING}-beta.{N} tags and returns max(N)+1 (min 1).
next_beta_counter() {
    upcoming_version="$1"

    # Highest existing beta counter for this upcoming version, or 0.
    highest=$(git tag -l "${upcoming_version}-beta.*" \
        | sed "s/^${upcoming_version}-beta\.\([0-9]*\)$/\1/" \
        | grep -E '^[0-9]+$' \
        | sort -n \
        | tail -1)

    if [ -z "${highest}" ]; then
        echo 1
    else
        echo $((highest + 1))
    fi
}

# Generate beta version: v{UPCOMING_VERSION}-beta.{N}
generate_beta_version() {
    upcoming_version="$1"
    counter="$2"
    echo "${upcoming_version}-beta.${counter}"
}

# Output JSON
output_json() {
    prod_version="$1"
    upcoming_version="$2"
    beta_version="$3"

    printf '{
  "prod_version": "%s",
  "upcoming_version": "%s",
  "beta_version": "%s"
}
' "${prod_version}" "${upcoming_version}" "${beta_version}"
}

# Main execution
main() {
    prod_version=$(get_prod_version)
    upcoming_version=$(increment_patch_version "${prod_version}")
    counter=$(next_beta_counter "${upcoming_version}")
    beta_version=$(generate_beta_version "${upcoming_version}" "${counter}")

    output_json "${prod_version}" "${upcoming_version}" "${beta_version}"
}

main
