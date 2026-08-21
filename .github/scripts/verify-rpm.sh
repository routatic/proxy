#!/usr/bin/env bash
# verify-rpm.sh — assert an RPM built by build-rpms.sh is actually correct.
#
# Every check below fails the script (and therefore the job) on mismatch. The
# full `rpm -qip` / `rpm -qlp` output is also printed for human review.
#
# Usage:
#   verify-rpm.sh <rpm-file> <raw-version> <expected-arch> <expected-elf-machine>
#
#   raw-version           version as passed to nfpm, e.g. 0.6.4 or 0.6.4-beta.1
#   expected-arch         x86_64 | aarch64
#   expected-elf-machine  ELF e_machine, little-endian hex: 3e00 (x86-64), b700 (AArch64)
#
# Requires rpm, rpm2cpio and cpio (present on ubuntu-latest; the caller should
# apt-install rpm if `command -v rpm` fails).
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <rpm-file> <raw-version> <expected-arch> <expected-elf-machine>" >&2
  exit 2
fi

RPM_FILE="$1"
RAW_VERSION="$2"
EXPECT_ARCH="$3"
EXPECT_MACHINE="$4"

EXPECT_NAME="routatic-proxy"
EXPECT_LICENSE="AGPL-3.0-only"

# nfpm's semver version_schema rewrites the prerelease separator so the RPM
# version sorts below the matching stable release: 0.6.4-beta.1 -> 0.6.4~beta.1
EXPECT_VERSION="$(printf '%s' "$RAW_VERSION" | tr '-' '~')"

FAILED=0
fail() {
  echo "::error::$1"
  FAILED=1
}

if [ ! -f "$RPM_FILE" ]; then
  echo "::error::$RPM_FILE does not exist"
  exit 1
fi

for TOOL in rpm rpm2cpio cpio; do
  if ! command -v "$TOOL" >/dev/null 2>&1; then
    echo "::error::required tool '$TOOL' not found on PATH"
    exit 1
  fi
done

echo "=============================================================="
echo "Verifying $(basename "$RPM_FILE")"
echo "  expect name=${EXPECT_NAME} version=${EXPECT_VERSION} arch=${EXPECT_ARCH}"
echo "=============================================================="

# ── Header metadata (printed in full, then asserted field by field) ──
echo "--- rpm -qip ---"
rpm -qip "$RPM_FILE"
echo

read -r GOT_NAME GOT_VERSION GOT_ARCH GOT_LICENSE <<EOF
$(rpm -qp --qf '%{NAME} %{VERSION} %{ARCH} %{LICENSE}\n' "$RPM_FILE")
EOF

[ "$GOT_NAME" = "$EXPECT_NAME" ] ||
  fail "Name mismatch: got '$GOT_NAME', want '$EXPECT_NAME'"
[ "$GOT_VERSION" = "$EXPECT_VERSION" ] ||
  fail "Version mismatch: got '$GOT_VERSION', want '$EXPECT_VERSION'"
[ "$GOT_ARCH" = "$EXPECT_ARCH" ] ||
  fail "Architecture mismatch: got '$GOT_ARCH', want '$EXPECT_ARCH'"
[ "$GOT_LICENSE" = "$EXPECT_LICENSE" ] ||
  fail "License mismatch: got '$GOT_LICENSE', want '$EXPECT_LICENSE'"

# ── Payload file list ──
echo "--- rpm -qlp ---"
rpm -qlp "$RPM_FILE"
echo

FILE_LIST="$(rpm -qlp "$RPM_FILE")"
for WANT in \
  /usr/bin/routatic-proxy \
  /etc/routatic-proxy/config.json \
  /usr/lib/systemd/user/routatic-proxy.service \
  /usr/share/licenses/routatic-proxy/LICENSE
do
  if printf '%s\n' "$FILE_LIST" | grep -Fxq "$WANT"; then
    echo "payload: $WANT ok"
  else
    fail "Payload is missing $WANT"
  fi
done

# ── Config file must be marked %config(noreplace) so upgrades never clobber
# local edits. rpm's fflags render that pair as "cn".
echo "--- rpm -qp --qf FILEFLAGS ---"
rpm -qp --qf '[%{FILENAMES} %{FILEFLAGS:fflags}\n]' "$RPM_FILE"
echo

CONFIG_FLAGS="$(rpm -qp --qf '[%{FILENAMES} %{FILEFLAGS:fflags}\n]' "$RPM_FILE" |
  awk '$1 == "/etc/routatic-proxy/config.json" { print $2 }')"
if [ "$CONFIG_FLAGS" = "cn" ]; then
  echo "config flags: cn (config|noreplace) ok"
else
  fail "/etc/routatic-proxy/config.json fflags: got '${CONFIG_FLAGS:-<none>}', want 'cn' (config|noreplace)"
fi

# ── Extract the payload and inspect the real binary ──
WORKDIR="$(mktemp -d)"
# shellcheck disable=SC2064  # expand WORKDIR now, not at trap time
trap "rm -rf '$WORKDIR'" EXIT

RPM_ABS="$(cd "$(dirname "$RPM_FILE")" && pwd)/$(basename "$RPM_FILE")"

# Two portability traps here, both found by running this on Fedora and Ubuntu:
#
#  1. --no-absolute-filenames: RPM payload members are absolute paths, and
#     whether cpio strips the leading "/" by default differs between
#     distributions. Without it, extraction targets the real /usr and /etc and
#     fails on permissions (or, as root, would overwrite the host).
#  2. Ubuntu's rpm2cpio exits 1 even on a fully successful extraction, while
#     Fedora's exits 0. Under `set -o pipefail` that sinks the whole pipeline,
#     so we judge cpio's status instead of the pipeline's — and then prove the
#     payload really is complete by comparing the extracted binary against the
#     size RPM recorded for it, rather than trusting either exit code.
# With pipefail off, the subshell's exit status is cpio's — the last command in
# the pipeline — which is the one whose success we actually care about.
CPIO_STATUS=0
set +o pipefail
(cd "$WORKDIR" && rpm2cpio "$RPM_ABS" | cpio -idm --quiet --no-absolute-filenames) ||
  CPIO_STATUS=$?
set -o pipefail

if [ "$CPIO_STATUS" -ne 0 ]; then
  fail "cpio failed to extract the RPM payload (exit $CPIO_STATUS)"
fi

EXPECTED_SIZE="$(rpm -qp --qf '[%{FILENAMES} %{FILESIZES}\n]' "$RPM_FILE" 2>/dev/null |
  awk '$1 == "/usr/bin/routatic-proxy" { print $2 }')"
ACTUAL_SIZE="$([ -f "${WORKDIR}/usr/bin/routatic-proxy" ] &&
  wc -c < "${WORKDIR}/usr/bin/routatic-proxy" | tr -d ' ' || echo 0)"
if [ -z "$EXPECTED_SIZE" ]; then
  fail "RPM header records no size for /usr/bin/routatic-proxy"
elif [ "$ACTUAL_SIZE" != "$EXPECTED_SIZE" ]; then
  fail "Extracted binary is truncated: got ${ACTUAL_SIZE} bytes, header says ${EXPECTED_SIZE}"
else
  echo "payload: extracted binary is complete (${ACTUAL_SIZE} bytes)"
fi

BIN="${WORKDIR}/usr/bin/routatic-proxy"
if [ ! -f "$BIN" ]; then
  fail "Extracted payload has no regular file at usr/bin/routatic-proxy"
else
  if [ -x "$BIN" ]; then
    echo "binary: executable ok ($(stat -c '%A' "$BIN"))"
  else
    fail "Packaged binary is not executable (mode $(stat -c '%A' "$BIN"))"
  fi

  # Read the ELF header directly rather than parsing `file` output, whose
  # wording differs between platforms. A guard that silently always passes is
  # worse than no guard.
  #   bytes 0-3   magic      7f 45 4c 46
  #   byte  4     class      02 = 64-bit
  #   bytes 18-19 e_machine  (LE) 3e00 = x86-64, b700 = AArch64
  HEADER=$(dd if="$BIN" bs=1 count=20 2>/dev/null | od -An -tx1 | tr -d ' \n')
  MAGIC="${HEADER:0:8}"
  CLASS="${HEADER:8:2}"
  MACHINE="${HEADER:36:4}"

  [ "$MAGIC" = "7f454c46" ] ||
    fail "Packaged binary is not an ELF file (magic=$MAGIC)"
  [ "$CLASS" = "02" ] ||
    fail "Packaged binary is not 64-bit ELF (class=$CLASS)"
  [ "$MACHINE" = "$EXPECT_MACHINE" ] ||
    fail "Packaged binary has wrong ELF machine: got $MACHINE, want $EXPECT_MACHINE"

  if [ "$MAGIC" = "7f454c46" ] && [ "$CLASS" = "02" ] && [ "$MACHINE" = "$EXPECT_MACHINE" ]; then
    echo "binary: ELF64 e_machine=$MACHINE ok"
  fi
fi

if [ "$FAILED" -ne 0 ]; then
  echo "::error::$(basename "$RPM_FILE") failed verification"
  exit 1
fi

echo "$(basename "$RPM_FILE"): all checks passed"
