#!/bin/bash
RUN_NAME=hertz_service
mkdir -p output/bin
go build -o output/bin/${RUN_NAME}
