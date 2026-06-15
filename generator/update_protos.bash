#!/bin/bash
# Selectively copy and normalize the Deadlock proto sources we decode from the
# SteamDatabase/Protobufs submodule into protocol/ as a single flat proto
# package named "protocol".
#
# The Source 2 engine wire format is shared across Source 2 games, so the engine
# protos (demo, net/svc messages, game events, base modifier) decode a Deadlock
# replay unchanged; only the Citadel user/game messages are game specific.
#
# Two normalizations make the set buildable with the reflect-free
# protobuf-go-lite generator, which does not support proto2 extensions:
#
#   1. Flatten every file into one proto package "protocol" and drop the
#      leading-dot fully-qualified type references so they resolve within it
#      (the go-dota2 generator does the same for the same Steam protos).
#   2. Strip metadata-only custom options and their defining "extend" blocks
#      (maximum_size_bytes, schema_friendly_name, network_connection tokens).
#      These annotate sizes/UI strings and never affect wire decoding, so the
#      parser does not need them. Dropping them also removes the
#      google/protobuf/descriptor.proto dependency entirely.
#
# Add proto files to PROTO_FILES as new message families are decoded. protoc
# resolves imports transitively, so a newly added proto may pull in a dependency
# that must also be listed here (and any unused heavy import stripped below).
set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SRC_DIR="${SCRIPT_DIR}/Protobufs/deadlock"
DST_DIR="${REPO_ROOT}/protocol"

# Module-relative import prefix. The aptre/common generator resolves proto
# imports against the vendored module tree (-I vendor), so intra-set imports use
# full module paths rather than bare filenames.
IMPORT_PREFIX="github.com/paralin/s2replay/protocol"

if [ ! -d "${SRC_DIR}" ]; then
	echo "missing proto submodule at ${SRC_DIR}" >&2
	echo "run: git submodule update --init --recursive" >&2
	exit 1
fi

# Deadlock proto allow-list. Keep this minimal: only protos whose messages the
# parser actually decodes, plus their transitive dependencies. Files that exist
# only to define custom options (network_connection, valveextensions) and the
# Steam GC econ universe (citadel_gcmessages_common and its steammessages /
# gcsdk / base_gcmessages imports) are deliberately excluded; see the import
# and field strips below.
PROTO_FILES=(
	demo.proto
	networkbasetypes.proto
	source2_steam_stats.proto
	netmessages.proto
	gameevents.proto
	usermessages.proto
	networksystem_protomessages.proto
	base_modifier.proto
	citadel_usermessages.proto
	citadel_gameevents.proto
)

WORK_DIR="$(mktemp -d)"
cleanup() { rm -rf "${WORK_DIR}"; }
trap cleanup EXIT

for proto in "${PROTO_FILES[@]}"; do
	if [ ! -f "${SRC_DIR}/${proto}" ]; then
		echo "missing source proto: ${proto}" >&2
		exit 1
	fi
	cp "${SRC_DIR}/${proto}" "${WORK_DIR}/${proto}"
done

for f in "${WORK_DIR}"/*.proto; do
	# Drop imports of files we exclude or no longer need after stripping
	# extensions: the Steam GC econ set, custom-option definition files, and the
	# well-known descriptor (only referenced by the extend blocks we remove).
	sed -i.bak -E \
		-e '/^import "(citadel_gcmessages_common|valveextensions|network_connection|google\/protobuf\/descriptor)\.proto";/d' \
		"$f"

	# Remove proto2 "extend" blocks (custom option definitions) and any
	# message/enum-level custom option usages, which protobuf-go-lite rejects.
	sed -i.bak -E \
		-e '/^extend [.]?google\.protobuf\./,/^}/d' \
		-e '/^[[:space:]]*option \(/d' \
		"$f"

	# Strip standalone bracketed custom options on enum values / fields, of the
	# form [(name) = value]. The only such options in the allow-list are
	# [(schema_friendly_name) = "..."]; built-in [default = ...] options carry no
	# parenthesis and are preserved.
	sed -i.bak -E -e 's/ \[\([^)]*\)[^]]*\]//g' "$f"

	# Drop the two map-ping fields whose types live in the excluded
	# citadel_gcmessages_common (CMsgLaneColor / CMsgMapLine); not on the decode
	# path.
	sed -i.bak -E -e '/^[[:space:]]*(optional|repeated)[[:space:]]+\.?(CMsgLaneColor|CMsgMapLine)[[:space:]]/d' "$f"

	# Rewrite the remaining bare intra-set imports to full module-relative paths
	# so the aptre generator resolves them against the vendored module tree.
	sed -i.bak -E -e "s#^import \"([a-z0-9_]+\.proto)\";#import \"${IMPORT_PREFIX}/\1\";#" "$f"

	# Prepend the shared package, then drop leading-dot qualifiers so the
	# previously root-scoped type references resolve within package protocol.
	# Nested qualifiers (Outer.Inner) keep their interior dots; only the leading
	# root dot is removed.
	body="$(sed -E \
		-e 's/(optional|repeated|required) \./\1 /g' \
		-e 's/\t\./\t/g' "$f")"
	{
		printf 'syntax = "proto2";\npackage protocol;\n\n'
		printf '%s\n' "$body"
	} >"${f}.norm"
	mv "${f}.norm" "$f"
	rm -f "${f}.bak"
done

mkdir -p "${DST_DIR}"
rm -f "${DST_DIR}"/*.proto
cp "${WORK_DIR}"/*.proto "${DST_DIR}/"

echo "copied ${#PROTO_FILES[@]} normalized proto files into protocol/"
