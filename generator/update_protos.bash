#!/bin/bash
# Selectively copy the Deadlock proto sources we decode from the
# SteamDatabase/Protobufs submodule into protocol/ as a flat package.
#
# Add proto files to PROTO_FILES as new message families are decoded. protoc
# resolves imports transitively, so a newly added proto may pull in a dependency
# that must also be listed here.
set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SRC_DIR="${SCRIPT_DIR}/Protobufs/deadlock"
DST_DIR="${REPO_ROOT}/protocol"

if [ ! -d "${SRC_DIR}" ]; then
	echo "missing proto submodule at ${SRC_DIR}" >&2
	echo "run: git submodule update --init --recursive" >&2
	exit 1
fi

# Deadlock proto allow-list. Keep this minimal: only protos whose messages the
# parser actually decodes, plus their transitive dependencies.
PROTO_FILES=(
	demo.proto
	networkbasetypes.proto
	netmessages.proto
	network_connection.proto
	gameevents.proto
	networksystem_protomessages.proto
	usermessages.proto
	citadel_usermessages.proto
	citadel_gameevents.proto
	citadel_gcmessages_common.proto
	base_modifier.proto
)

mkdir -p "${DST_DIR}"
rm -f "${DST_DIR}"/*.proto

for proto in "${PROTO_FILES[@]}"; do
	if [ ! -f "${SRC_DIR}/${proto}" ]; then
		echo "missing source proto: ${proto}" >&2
		exit 1
	fi
	cp "${SRC_DIR}/${proto}" "${DST_DIR}/${proto}"
done

echo "copied ${#PROTO_FILES[@]} proto files into protocol/"
