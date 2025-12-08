#!/bin/bash

set -e

wget -q -O /tmp/artifact.txt --header="Cookie: Authorization=$KOTH_ACCESS_TOKEN" "$KOTH_PUBLIC_FOLDER/artifact.txt"
echo $(cat /tmp/artifact.txt)