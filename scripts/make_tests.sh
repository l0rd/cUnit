#!/bin/sh

TARGET1="github.com/l0rd/docker-unit/build"
# TARGET2="github.com/l0rd/docker-unit/cmd/docker-unit"

export GOPATH="$PROJ_DIR/Godeps/_workspace:$GOPATH"

CMD="go test $TARGET1"

echo "$CMD" && $CMD
