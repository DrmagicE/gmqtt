#!/usr/bin/env bash

set -e
go test -race ./...  -coverprofile=coverage.txt -covermode=atomic
sed -i -e '/.pb./d' coverage.txt
sed -i -e '/_mock/d' coverage.txt
sed -i -e '/example/d' coverage.txt
sed -i -e '/_darwin/d' coverage.txt
sed -i -e '/_windows/d' coverage.txt